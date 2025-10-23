// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsman2_test

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	utilsnet "k8s.io/utils/net"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/app"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsprovider"
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
		mgrCancel       context.CancelFunc
		testRunID       string
		testNamespace   *corev1.Namespace
		firstZoneID     dns.ZoneID
		e1, e2, e3, e4  *v1alpha1.DNSEntry
		provider1       *v1alpha1.DNSProvider
		provider1Secret *corev1.Secret

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

		prepareSecondProvider = func() *v1alpha1.DNSProvider {
			mcfg := mock.MockConfig{
				Account: testRunID + "-2",
				Zones: []mock.MockZone{
					{DNSName: "other-domain.com"},
				},
			}
			firstZoneID = mcfg.Zones[0].ZoneID(testRunID)
			bytes, err := json.Marshal(&mcfg)
			Expect(err).NotTo(HaveOccurred())
			return &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testRunID,
					Name:      "mock2",
				},
				Spec: v1alpha1.DNSProviderSpec{
					Type:           "mock-inmemory",
					ProviderConfig: &runtime.RawExtension{Raw: bytes},
					// "mock1-secret" can be reused as it has no data anyway
					SecretRef: &corev1.SecretReference{Name: "mock1-secret", Namespace: testRunID},
				},
			}
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

		log.Info("Adding field indexes to informers")
		Expect(app.AddAllFieldIndexesToManager(ctx, mgr)).To(Succeed())

		By("Adding controllers to manager")
		if err := (&dnsprovider.Reconciler{
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

		provider1Secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "mock1-secret",
			},
			Data: map[string][]byte{},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(testClient.Create(ctx, provider1Secret)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, provider1Secret)).To(Succeed())
		})
		provider1 = &v1alpha1.DNSProvider{
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
		Expect(testClient.Create(ctx, provider1)).To(Succeed())
		DeferCleanup(func() {
			err := testClient.Delete(ctx, provider1)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
			Eventually(func(g Gomega) {
				g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), provider1)).NotTo(Succeed())
			}).Should(Succeed())
		})

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), provider1)).To(Succeed())
			g.Expect(provider1.Status.State).To(Equal("Ready"))
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
			g.Expect(e1.Status.Message).To(PointTo(Equal("failed to execute DNS change requests: 1 change failed")))
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

	It("should set state of invalid entries to invalid", func() {
		By("Create new DNS entry with both targets and text specified")
		e := &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "e-both",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "e-both.first.example.com",
				Targets: []string{"1.1.1.1"},
				Text:    []string{"foo"},
			},
		}
		Expect(testClient.Create(ctx, e)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, e)).To(Succeed())
		})

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e), e)).To(Succeed())
			g.Expect(e.Finalizers).To(BeEmpty())
			g.Expect(e.Status.State).To(Equal("Invalid"))
			g.Expect(e.Status.Message).To(PointTo(ContainSubstring("cannot specify both targets and text fields")))
			g.Expect(e.Status.ObservedGeneration).To(Equal(e.Generation))
		}).Should(Succeed())
	})

	It("should set state of entry without matching provider to error", func() {
		By("Create new DNS entry with unknown dns name domain")
		e := &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "e-unknown-domain",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "e.unknown.com",
				Targets: []string{"1.1.1.1"},
			},
		}
		Expect(testClient.Create(ctx, e)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, e)).To(Succeed())
		})

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e), e)).To(Succeed())
			g.Expect(e.Finalizers).To(BeEmpty())
			g.Expect(e.Status.State).To(Equal("Error"))
			g.Expect(e.Status.Message).To(PointTo(ContainSubstring("no matching DNS provider found")))
			g.Expect(e.Status.ObservedGeneration).To(Equal(e.Generation))
		}).Should(Succeed())
	})

	It("should not delete provider until all its entries have been deleted", func() {
		Expect(testClient.Create(ctx, e1)).To(Succeed())
		checkEntry(e1)

		By("Try to delete provider")
		Expect(testClient.Delete(ctx, provider1)).To(Succeed())

		Eventually(func(g Gomega) {
			p := &v1alpha1.DNSProvider{}
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), p)).To(Succeed())
			g.Expect(p.DeletionTimestamp).NotTo(BeNil())
			g.Expect(p.Status.Message).To(PointTo(Equal("cannot delete provider, 1 DNSEntries still assigned to it")))
		}).Should(Succeed())

		By("Delete entry")
		Expect(testClient.Delete(ctx, e1)).To(Succeed())
		Eventually(func(g Gomega) {
			checkDeleted(g, ctx, e1)
		}).Should(Succeed())

		By("Await deletion of provider")
		Eventually(func(g Gomega) {
			p := &v1alpha1.DNSProvider{}
			g.Expect(errors.IsNotFound(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), p))).To(BeTrue())
		}).Should(Succeed())
	})

	It("should reassign entry to different provider if dnsName changes", func() {
		By("Create second provider")
		p2 := prepareSecondProvider()
		Expect(testClient.Create(ctx, p2)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, p2)).To(Succeed())
		})

		By("Create entry in domain of first provider")
		Expect(testClient.Create(ctx, e1)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, e1)).To(Succeed())
		})

		checkEntry(e1)
		Expect(e1.Finalizers).To(ContainElement(dns.FinalizerCompound))
		Expect(e1.Status.Provider).To(PointTo(Equal(client.ObjectKeyFromObject(provider1).String())))
		Expect(e1.Status.ProviderType).To(PointTo(Equal("mock-inmemory")))

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(p2), p2)).To(Succeed())
			g.Expect(p2.Status.State).To(Equal("Ready"))
		}).Should(Succeed())

		By("Update entry to domain of second provider")
		e1.Spec.DNSName = "e1.other-domain.com"
		Expect(testClient.Update(ctx, e1)).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
			g.Expect(e1.Status.ObservedGeneration).To(Equal(e1.Generation))
			g.Expect(e1.Finalizers).To(ContainElement(dns.FinalizerCompound))
			g.Expect(e1.Status.Provider).To(PointTo(Equal(client.ObjectKeyFromObject(p2).String())))
			g.Expect(e1.Status.ProviderType).To(PointTo(Equal("mock-inmemory")))
		}).Should(Succeed())
		checkEntry(e1)
	})

	It("should handle an entry with multiple cname targets/resolveTargetsToAddresses correctly", func() {
		By("Create new DNS entry with multiple cname targets")
		entry := &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "e-multi-cname",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "e-multi-cname.first.example.com",
				Targets: []string{"wikipedia.org", "www.wikipedia.org", "gardener.cloud"},
			},
		}

		Expect(testClient.Create(ctx, entry)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, entry)).To(Succeed())
		})

		By("Check entry is ready and all targets are resolved to addresses")
		// Note: wikipedia.org has both ipv4 and ipv6 addresses, gardener.cloud has multiple ipv4 and ipv6 addresses
		// www.wikipedia.org resolves to wikipedia.org and checks for duplicate addresses are done
		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(entry), entry)).To(Succeed())
			g.Expect(entry.Status.State).To(Equal("Ready"))
			g.Expect(len(entry.Status.Targets)).To(BeNumerically(">=", len(entry.Spec.Targets)))
			var countIPV4, countIPV6 int
			for _, t := range entry.Status.Targets {
				ip := net.ParseIP(t)
				if ip != nil && utilsnet.IsIPv4(ip) {
					countIPV4++
				} else if ip != nil && utilsnet.IsIPv6(ip) {
					countIPV6++
				}
			}
			g.Expect(countIPV4).To(BeNumerically(">=", len(entry.Spec.Targets)))
			g.Expect(countIPV6).To(BeNumerically(">=", len(entry.Spec.Targets)))
			g.Expect(entry.Status.ObservedGeneration).To(Equal(entry.Generation))
		}).Should(Succeed())

		// check for duplicates of wikipedia.org and www.wikipedia.org addresses
		addresses := make(map[string]struct{})
		for _, t := range entry.Status.Targets {
			_, exists := addresses[t]
			Expect(exists).To(BeFalse(), "duplicate address "+t)
			addresses[t] = struct{}{}
		}
	})

	It("should not delete a stale entry", func() {
		Expect(testClient.Create(ctx, e1)).To(Succeed())
		checkEntry(e1)

		By("change the provider to make the entry stale")
		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), provider1)).To(Succeed())
		provider1.Spec.Domains = &v1alpha1.DNSSelection{Include: []string{"restricted.first.example.com"}}
		Expect(testClient.Update(ctx, provider1)).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), provider1)).To(Succeed())
			g.Expect(provider1.Status.ObservedGeneration).To(Equal(provider1.Generation))
			g.Expect(provider1.Status.State).To(Equal("Ready"))
		}).Should(Succeed())

		By("check that the entry is now stale")
		// entry should be stale as its domain is not covered by the provider anymore
		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
			g.Expect(e1.Status.State).To(Equal("Stale"))
		}).Should(Succeed())

		By("Try to delete entry and ensure it is not gone")
		Expect(testClient.Delete(ctx, e1)).To(Succeed())
		Consistently(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
			g.Expect(e1.Status.State).To(Equal("Stale"))
		}, 2*time.Second, 500*time.Second).Should(Succeed())

		By("Revert provider change")
		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), provider1)).To(Succeed())
		provider1.Spec.Domains = nil
		Expect(testClient.Update(ctx, provider1)).To(Succeed())

		By("check that the entry is deleted now")
		Eventually(func(g Gomega) {
			g.Expect(errors.IsNotFound(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1))).To(BeTrue())
		}).Should(Succeed())
	})

	It("should respect the provider rate limits", func() {
		By("Create second provider")
		p2 := prepareSecondProvider()
		p2.Spec.RateLimit = &v1alpha1.RateLimit{
			RequestsPerDay: 60 * 60 * 24, // 2 per 1s
			Burst:          1,
		}
		Expect(testClient.Create(ctx, p2)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, p2)).To(Succeed())
		})

		By("Create three entries in domain of second provider")
		// create 3 entries that all get assigned to the second provider
		// with a rate limit of 1/s this should take at least 2s to create all entries
		entries := make([]*v1alpha1.DNSEntry, 3)
		for i := 0; i < len(entries); i++ {
			entries[i] = &v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testRunID,
					Name:      fmt.Sprintf("e-rate-%d", i),
				},
				Spec: v1alpha1.DNSEntrySpec{
					DNSName: fmt.Sprintf("e-rate-%d.other-domain.com", i),
					Targets: []string{fmt.Sprintf("2.2.2.%d", i)},
				},
			}
			Expect(testClient.Create(ctx, entries[i])).To(Succeed())
			DeferCleanup(func() {
				Expect(testClient.Delete(ctx, entries[i])).To(Succeed())
			})
		}

		start := time.Now()
		for _, entry := range entries {
			checkEntry(entry)
		}
		duration := time.Since(start)
		Expect(duration).To(BeNumerically(">=", 2*time.Second), "creating 3 entries with rate limit of 1/s should take at least 2s, took %s", duration)
	})

	It("should update provider and entries when provider secret is created after provider resource", func() {
		By("Create second provider")
		p2 := prepareSecondProvider()
		// initially set secret ref to non-existing secret
		p2.Spec.SecretRef = &corev1.SecretReference{Name: "mock2-secret", Namespace: testRunID}
		Expect(testClient.Create(ctx, p2)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, p2)).To(Succeed())
		})

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(p2), p2)).To(Succeed())
			g.Expect(p2.Status.State).To(Equal("Error"))
		}).Should(Succeed())

		By("Create an entry assigned to the second provider")
		entry := &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "e2",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "e2.other-domain.com",
				Targets: []string{"2.2.2.2"},
			},
		}
		Expect(testClient.Create(ctx, entry)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, entry)).To(Succeed())
		})

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(entry), entry)).To(Succeed())
			g.Expect(entry.Status.State).To(Equal("Error"))
		}).Should(Succeed())

		By("create secret")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "mock2-secret",
			},
			Data: map[string][]byte{},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(testClient.Create(ctx, secret)).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(p2), p2)).To(Succeed())
			g.Expect(p2.Status.State).To(Equal("Ready"))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(entry), entry)).To(Succeed())
			g.Expect(entry.Status.State).To(Equal("Ready"))
		}).Should(Succeed())
	})

	It("should add and remove finalizer for provider secret", func() {
		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1Secret), provider1Secret)).To(Succeed())
		Expect(provider1Secret.Finalizers).To(ContainElement(dns.FinalizerCompound))

		By("Replace secret")
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "mock1b-secret",
			},
			Data: map[string][]byte{},
			Type: corev1.SecretTypeOpaque,
		}
		Expect(testClient.Create(ctx, newSecret)).To(Succeed())
		DeferCleanup(func() {
			Expect(testClient.Delete(ctx, newSecret)).To(Succeed())
		})

		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), provider1)).To(Succeed())
		provider1.Spec.SecretRef = &corev1.SecretReference{Name: "mock1b-secret", Namespace: testRunID}
		Expect(testClient.Update(ctx, provider1)).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), provider1)).To(Succeed())
			g.Expect(provider1.Status.ObservedGeneration).To(Equal(provider1.Generation))
			g.Expect(provider1.Status.State).To(Equal("Ready"))
		}).Should(Succeed())

		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1Secret), provider1Secret)).To(Succeed())
		Expect(provider1Secret.Finalizers).NotTo(ContainElement(dns.FinalizerCompound))

		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(newSecret), newSecret)).To(Succeed())
		Expect(newSecret.Finalizers).To(ContainElement(dns.FinalizerCompound))

		By("Delete provider")
		Expect(testClient.Delete(ctx, provider1)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(errors.IsNotFound(testClient.Get(ctx, client.ObjectKeyFromObject(provider1), provider1))).To(BeTrue())
		}).Should(Succeed())

		By("Check provider secret has no finalizer anymore")
		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider1Secret), provider1Secret)).To(Succeed())
		Expect(provider1Secret.Finalizers).NotTo(ContainElement(dns.FinalizerCompound))
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
