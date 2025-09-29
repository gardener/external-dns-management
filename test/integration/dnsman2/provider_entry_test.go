// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsman2_test

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/app"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry"
	dnsprovidercontrolplane "github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsprovider/controlplane"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/mock"
)

var debug = false

var _ = Describe("Provider/Entry collaboration tests", func() {
	const (
		defaultTTL   = 300
		retryTimeout = 5 * time.Second
	)

	var (
		mgrCancel      context.CancelFunc
		testRunID      string
		testNamespace  *corev1.Namespace
		firstZoneID    dns.ZoneID
		e1, e2, e3, e4 *v1alpha1.DNSEntry

		checkDeleted = func(g Gomega, ctx context.Context, entry *v1alpha1.DNSEntry) {
			err := testClient.Get(ctx, client.ObjectKeyFromObject(entry), entry)
			g.ExpectWithOffset(1, err).To(HaveOccurred())
			g.ExpectWithOffset(1, client.IgnoreNotFound(err)).To(Succeed())
		}

		checkSingleEntryInMockDatabase = func(entry *v1alpha1.DNSEntry) {
			dump := mock.GetInMemoryMock(testRunID).BuildFullDump()
			for _, zoneDump := range dump.InMemory {
				switch {
				case zoneDump.HostedZone.Domain == "first.example.com" && entry == nil:
					ExpectWithOffset(1, zoneDump.DNSSets).To(BeEmpty(), "unexpected number of DNS sets in first.example.com")
				case zoneDump.HostedZone.Domain == "first.example.com" && entry != nil:
					ExpectWithOffset(1, zoneDump.DNSSets).To(HaveKey(dns.DNSSetName{DNSName: entry.Spec.DNSName}))
					ExpectWithOffset(1, zoneDump.DNSSets).To(HaveLen(1), "unexpected number of DNS sets in first.example.com")
					set := zoneDump.DNSSets[dns.DNSSetName{DNSName: entry.Spec.DNSName}]
					ExpectWithOffset(1, set.Sets).To(HaveKey(dns.TypeA))
					ExpectWithOffset(1, set.Sets).To(HaveLen(1))
					setA := set.Sets[dns.TypeA]
					ExpectWithOffset(1, setA.Records).To(ConsistOf(
						PointTo(MatchFields(IgnoreExtras, Fields{
							"Value": Equal(entry.Spec.Targets[0]),
						}))))
					ExpectWithOffset(1, setA.Type).To(Equal(dns.TypeA))
					ExpectWithOffset(1, setA.TTL).To(Equal(int64(defaultTTL)))
				case zoneDump.HostedZone.Domain == "second.example.com":
					ExpectWithOffset(1, zoneDump.DNSSets).To(BeEmpty(), "unexpected number of DNS sets in second.example.com")
				default:
					Fail("unexpected zone domain " + zoneDump.HostedZone.Domain)
				}
			}
		}

		checkEntry = func(entry *v1alpha1.DNSEntry) {
			Eventually(func(g Gomega) {
				g.ExpectWithOffset(1, testClient.Get(ctx, client.ObjectKeyFromObject(entry), entry)).To(Succeed())
				g.ExpectWithOffset(1, entry.Status.State).To(Equal("Ready"))
				if entry.Spec.Targets != nil {
					g.ExpectWithOffset(1, entry.Status.Targets).To(Equal(entry.Spec.Targets))
				} else {
					g.ExpectWithOffset(1, entry.Status.Targets).To(Equal(quoted(entry.Spec.Text)))
				}
				if entry.Spec.TTL != nil {
					g.ExpectWithOffset(1, entry.Status.TTL).To(Equal(entry.Spec.TTL))
				} else {
					g.ExpectWithOffset(1, entry.Status.TTL).To(Equal(ptr.To[int64](defaultTTL)))
				}
				g.ExpectWithOffset(1, entry.Status.ObservedGeneration).To(Equal(entry.Generation))
			}).Should(Succeed())
		}
	)

	BeforeEach(func() {
		if debug {
			SetDefaultEventuallyTimeout(30 * time.Second)
		}

		By("Create test Namespace")
		testNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "dnsman2-",
			},
		}
		Expect(testClient.Create(ctx, testNamespace)).To(Succeed())
		log.Info("Created Namespace for test", "namespaceName", testNamespace.Name)
		testRunID = testNamespace.Name

		DeferCleanup(func() {
			By("Delete test Namespace")
			Expect(testClient.Delete(ctx, testNamespace)).To(Or(Succeed(), matchers.BeNotFoundError()))
		})

		cfg := &config.DNSManagerConfiguration{
			LogLevel:  "debug",
			LogFormat: "text",
			Controllers: config.ControllerConfiguration{
				DNSProvider: config.DNSProviderControllerConfig{
					Namespace:                 testRunID,
					DefaultTTL:                ptr.To[int64](300),
					AllowMockInMemoryProvider: ptr.To(true),
					SkipNameValidation:        ptr.To(true),
				},
				DNSEntry: config.DNSEntryControllerConfig{
					ReconciliationDelayAfterUpdate: ptr.To(metav1.Duration{Duration: 10 * time.Millisecond}),
					SkipNameValidation:             ptr.To(true),
				},
			},
		}
		cfg.LeaderElection.LeaderElect = false

		By("setting up manager")
		mgr, err := manager.New(restConfig, manager.Options{
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
			Logger:                  log,
			Scheme:                  dnsmanclient.ClusterScheme,
			GracefulShutdownTimeout: ptr.To(5 * time.Second),
			Cache: cache.Options{
				// TODO(MartinWeindel) Revisit this, when introducing flag to allow DNSProvider in all namespaces
				ByObject: map[client.Object]cache.ByObject{
					&corev1.Secret{}: {
						Namespaces: map[string]cache.Config{cfg.Controllers.DNSProvider.Namespace: {}},
					},
				},
			},
		})
		Expect(err).ToNot(HaveOccurred())

		//Expect(apiextensionsv1.AddToScheme(mgr.GetScheme())).To(Succeed())
		//Expect(v1alpha1.AddToScheme(mgr.GetScheme())).To(Succeed())

		log.Info("Adding field indexes to informers")
		Expect(app.AddAllFieldIndexesToManager(ctx, mgr)).To(Succeed())

		By("Adding controllers to manager")
		if err := (&dnsprovidercontrolplane.Reconciler{
			Config: *cfg,
		}).AddToManager(mgr, mgr); err != nil {
			Fail(fmt.Errorf("failed adding control plane DNSProvider controller: %w", err).Error())
		}
		if err := (&dnsentry.Reconciler{
			Config:    cfg.Controllers.DNSEntry,
			Namespace: cfg.Controllers.DNSProvider.Namespace,
		}).AddToManager(mgr, mgr); err != nil {
			Fail(fmt.Errorf("failed adding control plane DNSEntry controller: %w", err).Error())
		}

		var mgrContext context.Context
		mgrContext, mgrCancel = context.WithCancel(ctx)

		By("starting manager")
		go func() {
			defer GinkgoRecover()
			err := mgr.Start(mgrContext)
			Expect(err).NotTo(HaveOccurred())
		}()

		DeferCleanup(func() {
			By("stopping manager")
			mgrCancel()
		})

		mcfg := mock.MockConfig{
			Account: testRunID,
			Zones: []mock.MockZone{
				{DNSName: "first.example.com"},
				{DNSName: "second.example.com"},
			},
		}
		firstZoneID = mcfg.Zones[0].ZoneID(testRunID)
		bytes, err := json.Marshal(&mcfg)
		Expect(err).NotTo(HaveOccurred())

		providerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "mock1-secret",
			},
			Data: map[string][]byte{},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(testClient.Create(ctx, providerSecret)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, providerSecret)).To(Succeed())
			println("Deleted provider secret")
		})
		provider := &v1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "mock1",
			},
			Spec: v1alpha1.DNSProviderSpec{
				Type:           "mock-inmemory",
				ProviderConfig: &runtime.RawExtension{Raw: bytes},
				SecretRef:      &corev1.SecretReference{Name: "mock1-secret", Namespace: testRunID},
			},
		}
		Expect(testClient.Create(ctx, provider)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, provider)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider), provider)).NotTo(Succeed())
			}).Should(Succeed())
		})

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider), provider)).To(Succeed())
			g.Expect(provider.Status.State).To(Equal("Ready"))
		}).Should(Succeed())

		e1 = &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "e1",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "e1.first.example.com",
				Targets: []string{"1.1.1.1"},
			},
		}
		e2 = &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "e2",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "e2.first.example.com",
				Targets: []string{"1.1.2.1", "1.1.2.2", "1::2"},
				TTL:     ptr.To[int64](42),
			},
		}
		e3 = &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "e3",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "e3.second.example.com",
				Text:    []string{"foo bar", "blabla"},
			},
		}
		e4 = &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "e4",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "e4.first.example.com",
				Targets: []string{"wikipedia.org"},
			},
		}
	})

	It("should create and update entries", func() {
		for _, entry := range []*v1alpha1.DNSEntry{e1, e2, e3, e4} {
			Expect(testClient.Create(ctx, entry)).To(Succeed())
		}

		for _, entry := range []*v1alpha1.DNSEntry{e1, e2, e3, e4} {
			By("entry " + entry.Name)
			checkEntry(entry)
		}

		e1.Spec.DNSName = "e1-update.first.example.com"
		e2.Spec.Targets = []string{"1.1.2.10", "1.1.2.2", "1::20"}
		e3.Spec.Text = []string{"foo bar2", "blabla2"}
		e4.Spec.Targets = []string{"1.1.1.1"}

		for _, entry := range []*v1alpha1.DNSEntry{e1, e2, e3, e4} {
			Expect(testClient.Update(ctx, entry)).To(Succeed())
		}

		for _, entry := range []*v1alpha1.DNSEntry{e1, e2, e3, e4} {
			By("entry " + entry.Name)
			checkEntry(entry)
		}

		for _, entry := range []*v1alpha1.DNSEntry{e1, e2, e3} {
			Expect(testClient.Delete(ctx, entry)).To(Succeed())
		}

		for _, entry := range []*v1alpha1.DNSEntry{e1, e2, e3} {
			By("await deletion of entry " + entry.Name)
			Eventually(func(g Gomega) {
				checkDeleted(g, ctx, entry)
			}).Should(Succeed())
		}

		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e4), e4)).To(Succeed())

		By("check mock database")
		checkSingleEntryInMockDatabase(e4)

		By("await deletion of entry " + e4.Name)
		Expect(testClient.Delete(ctx, e4)).To(Succeed())
		Eventually(func(g Gomega) {
			checkDeleted(g, ctx, e4)
		}).Should(Succeed())
	})

	It("should deal with temporary backend failures on creating an entry", func() {
		// simulate apply failure for entry e1
		failSet := dns.NewDNSSet(dns.DNSSetName{DNSName: e1.Spec.DNSName})
		failSet.Sets.AddRecord(dns.TypeA, e1.Spec.Targets[0], defaultTTL)
		failID := mock.GetInMemoryMock(testRunID).AddApplyFailSimulation(firstZoneID, &provider.ChangeRequests{
			Name: failSet.Name,
			Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
				dns.TypeA: {
					New: failSet.Sets[dns.TypeA],
				},
			},
		})

		Expect(testClient.Create(ctx, e1)).To(Succeed())

		Eventually(func() int {
			return mock.GetInMemoryMock(testRunID).GetApplyFailSimulationCount(failID)
		}).ShouldNot(BeZero())

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
			g.Expect(e1.Status.State).To(Equal("Error"))
			g.Expect(e1.Status.Message).To(PointTo(Equal("1 change failed")))
			g.Expect(e1.Status.ObservedGeneration).To(Equal(e1.Generation))
		}).Should(Succeed())

		mock.GetInMemoryMock(testRunID).RemoveApplyFailSimulation(failID)

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
			g.Expect(e1.Status.State).To(Equal("Ready"))
			g.Expect(e1.Status.ObservedGeneration).To(Equal(e1.Generation))
		}).WithTimeout(retryTimeout).Should(Succeed())

		By("check mock database")
		checkSingleEntryInMockDatabase(e1)

		Expect(testClient.Delete(ctx, e1)).To(Succeed())
		Eventually(func(g Gomega) {
			checkDeleted(g, ctx, e1)
		}).Should(Succeed())
	})

	It("should deal with temporary backend failures on updating an entry", func() {
		Expect(testClient.Create(ctx, e1)).To(Succeed())

		checkEntry(e1)

		newDNSName := "e1-update.first.example.com"
		failSet := dns.NewDNSSet(dns.DNSSetName{DNSName: newDNSName})
		failSet.Sets.AddRecord(dns.TypeA, e1.Spec.Targets[0], defaultTTL)
		failID := mock.GetInMemoryMock(testRunID).AddApplyFailSimulation(firstZoneID, &provider.ChangeRequests{
			Name: failSet.Name,
			Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
				dns.TypeA: {
					New: failSet.Sets[dns.TypeA],
				},
			},
		})

		Eventually(func() error {
			if err := testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1); err != nil {
				return err
			}
			// simulate apply failure for entry e1
			e1.Spec.DNSName = newDNSName
			return testClient.Update(ctx, e1)
		}).Should(Succeed())

		Eventually(func() int {
			return mock.GetInMemoryMock(testRunID).GetApplyFailSimulationCount(failID)
		}).ShouldNot(BeZero())

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
			g.Expect(e1.Status.State).To(Equal("Error"))
			g.Expect(e1.Status.ObservedGeneration).To(Equal(e1.Generation))
		}).Should(Succeed())

		mock.GetInMemoryMock(testRunID).RemoveApplyFailSimulation(failID)

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
			g.Expect(e1.Status.State).To(Equal("Ready"))
			g.Expect(e1.Status.ObservedGeneration).To(Equal(e1.Generation))
		}).WithTimeout(retryTimeout).Should(Succeed())

		By("check mock database")
		checkSingleEntryInMockDatabase(e1)

		Expect(testClient.Delete(ctx, e1)).To(Succeed())
		Eventually(func(g Gomega) {
			checkDeleted(g, ctx, e1)
		}).Should(Succeed())
	})

	It("should deal with temporary backend failures on deleting an entry", func() {
		Expect(testClient.Create(ctx, e2)).To(Succeed())
		checkEntry(e2)

		// simulate apply failure for entry e2
		deleteSet := dns.NewDNSSet(dns.DNSSetName{DNSName: "e2.first.example.com"})
		deleteSet.Sets.AddRecord(dns.TypeA, "1.1.2.1", 42)
		deleteSet.Sets.AddRecord(dns.TypeA, "1.1.2.2", 42)
		failID := mock.GetInMemoryMock(testRunID).AddApplyFailSimulation(firstZoneID, &provider.ChangeRequests{
			Name: deleteSet.Name,
			Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
				dns.TypeA: {
					Old: deleteSet.Sets[dns.TypeA],
				},
			},
		})
		Expect(testClient.Delete(ctx, e2)).To(Succeed())

		Eventually(func() int {
			return mock.GetInMemoryMock(testRunID).GetApplyFailSimulationCount(failID)
		}).ShouldNot(BeZero())

		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e2), e2)).To(Succeed())
		Expect(e2.DeletionTimestamp).NotTo(BeNil())

		// remove apply fail simulation
		mock.GetInMemoryMock(testRunID).RemoveApplyFailSimulation(failID)
		By("await deletion of entry " + e2.Name)
		Eventually(func(g Gomega) {
			checkDeleted(g, ctx, e2)
		}).WithTimeout(retryTimeout).Should(Succeed())

		By("check mock database")
		checkSingleEntryInMockDatabase(nil)
	})

	It("should remove the Gardener reconcile operation annotation after reconciliation", func() {
		By("Create new DNS entry")
		Expect(testClient.Create(ctx, e1)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, e1)).To(Succeed())
		})
		checkEntry(e1)

		By("Set reconcile annotation on DNS entry")
		e1.Annotations = map[string]string{
			constants.GardenerOperation: constants.GardenerOperationReconcile,
		}
		Expect(testClient.Update(ctx, e1)).To(Succeed())

		By("Wait for the reconcile annotation to be removed from the DNS entry")
		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
			g.Expect(e1.Annotations).NotTo(HaveKey(constants.GardenerOperation))
		}).Should(Succeed())
	})
})

func quoted(txt []string) []string {
	if txt == nil {
		return nil
	}
	quoted := make([]string, len(txt))
	for i, s := range txt {
		quoted[i] = strconv.Quote(s)
	}
	return quoted
}
