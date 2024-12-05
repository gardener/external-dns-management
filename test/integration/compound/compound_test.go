/*
 * // SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 * //
 * // SPDX-License-Identifier: Apache-2.0
 */

package compound_test

import (
	"context"
	"strconv"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/provider/mock"
)

var _ = Describe("Compound controller tests", func() {
	const (
		defaultTTL   = 300
		retryTimeout = 5 * time.Second
	)

	var (
		testRunID      string
		testNamespace  *corev1.Namespace
		firstZoneID    dns.ZoneID
		e1, e2, e3, e4 *v1alpha1.DNSEntry

		checkDeleted = func(g Gomega, ctx context.Context, entry *v1alpha1.DNSEntry) {
			err := testClient.Get(ctx, client.ObjectKeyFromObject(entry), entry)
			g.Expect(err).To(HaveOccurred())
			g.Expect(client.IgnoreNotFound(err)).To(Succeed())
		}

		checkSingleEntryInMockDatabaseGomega = func(g Gomega, entry *v1alpha1.DNSEntry) {
			dump := mock.TestMock[testRunID].BuildFullDump()
			for _, zoneDump := range dump.InMemory {
				switch {
				case zoneDump.HostedZone.Domain == "first.example.com" && entry == nil:
					g.Expect(zoneDump.DNSSets).To(BeEmpty(), "unexpected number of DNS sets in first.example.com")
				case zoneDump.HostedZone.Domain == "first.example.com" && entry != nil:
					g.Expect(zoneDump.DNSSets).To(HaveKey(dns.DNSSetName{DNSName: entry.Spec.DNSName}))
					g.Expect(zoneDump.DNSSets).To(HaveLen(1), "unexpected number of DNS sets in first.example.com")
					set := zoneDump.DNSSets[dns.DNSSetName{DNSName: entry.Spec.DNSName}]
					g.Expect(set.Sets).To(HaveKey("A"))
					g.Expect(set.Sets).To(HaveKey("META"))
					g.Expect(set.Sets).To(HaveLen(2))
					setA := set.Sets["A"]
					g.Expect(setA.Records).To(ConsistOf(
						PointTo(MatchFields(IgnoreExtras, Fields{
							"Value": Equal(entry.Spec.Targets[0]),
						}))))
					g.Expect(setA.Type).To(Equal("A"))
					g.Expect(setA.TTL).To(Equal(int64(defaultTTL)))
				case zoneDump.HostedZone.Domain == "second.example.com":
					g.Expect(zoneDump.DNSSets).To(BeEmpty(), "unexpected number of DNS sets in second.example.com")
				default:
					g.Expect(false).To(BeTrue(), "unexpected zone domain "+zoneDump.HostedZone.Domain)
				}
			}
		}

		checkSingleEntryInMockDatabase = func(entry *v1alpha1.DNSEntry) {
			checkSingleEntryInMockDatabaseGomega(Default, entry)
		}

		checkEntry = func(entry *v1alpha1.DNSEntry) {
			Eventually(func(g Gomega) {
				Expect(testClient.Get(ctx, client.ObjectKeyFromObject(entry), entry)).To(Succeed())
				g.Expect(entry.Status.State).To(Equal("Ready"))
				if entry.Spec.Targets != nil {
					g.Expect(entry.Status.Targets).To(Equal(entry.Spec.Targets))
				} else {
					g.Expect(entry.Status.Targets).To(Equal(quoted(entry.Spec.Text)))
				}
				if entry.Spec.TTL != nil {
					g.Expect(entry.Status.TTL).To(Equal(entry.Spec.TTL))
				} else {
					g.Expect(entry.Status.TTL).To(Equal(ptr.To[int64](defaultTTL)))
				}
				g.Expect(entry.Status.ObservedGeneration).To(Equal(entry.Generation))
			}).Should(Succeed())
		}
	)

	BeforeEach(func() {
		//SetDefaultEventuallyTimeout(30 * time.Second)

		ctxLocal := context.Background()
		ctx0 := ctxutil.CancelContext(ctxutil.WaitGroupContext(context.Background(), "main"))
		ctx = ctxutil.TickContext(ctx0, controllermanager.DeletionActivity)

		By("Create test Namespace")
		testNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "compound-",
			},
		}
		Expect(testClient.Create(ctxLocal, testNamespace)).To(Succeed())
		log.Info("Created Namespace for test", "namespaceName", testNamespace.Name)
		testRunID = testNamespace.Name

		DeferCleanup(func() {
			By("Delete test Namespace")
			Expect(testClient.Delete(ctxLocal, testNamespace)).To(Or(Succeed(), BeNotFoundError()))
		})

		By("Start manager")

		go func() {
			defer GinkgoRecover()
			args := []string{
				"--kubeconfig", kubeconfigFile,
				"--controllers", "dnscontrollers,annotation",
				"--omit-lease",
				"--dns-delay", "1s",
				"--reschedule-delay", "2s",
				"--lock-status-check-period", "5s",
				"--pool.size", "2",
			}
			runControllerManager(ctx, args)
		}()

		DeferCleanup(func() {
			By("Stop manager")
			if ctx != nil {
				ctxutil.Cancel(ctx)
			}
		})

		mcfg := mock.MockConfig{
			Name: testRunID,
			Zones: []mock.MockZone{
				{ZonePrefix: testRunID + ":first:", DNSName: "first.example.com"},
				{ZonePrefix: testRunID + ":second:", DNSName: "second.example.com"},
			},
		}
		firstZoneID = mcfg.Zones[0].ZoneID()
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
		})

		Eventually(func(g Gomega) {
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(provider), provider)).To(Succeed())
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

		Expect(testClient.Delete(ctx, e4)).To(Succeed())
		Eventually(func(g Gomega) {
			checkDeleted(g, ctx, e4)
		}).Should(Succeed())
	})

	It("should deal with temporary backend failures on creating an entry", func() {
		// simulate apply failure for entry e1
		failSet := dns.NewDNSSet(dns.DNSSetName{DNSName: e1.Spec.DNSName}, nil)
		failSet.UpdateGroup = testRunID
		failSet.Sets.AddRecord("A", e1.Spec.Targets[0], defaultTTL)
		failID := mock.TestMock[testRunID].AddApplyFailSimulation(firstZoneID, &provider.ChangeRequest{
			Action:   provider.R_CREATE,
			Type:     "A",
			Addition: failSet,
		})

		Expect(testClient.Create(ctx, e1)).To(Succeed())

		Eventually(func() int {
			return mock.TestMock[testRunID].GetApplyFailSimulationCount(failID)
		}).ShouldNot(BeZero())

		Eventually(func(g Gomega) {
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
			g.Expect(e1.Status.State).To(Or(Equal("Error"), Equal("Stale")))
			g.Expect(e1.Status.ObservedGeneration).To(Equal(e1.Generation))
		}).Should(Succeed())

		mock.TestMock[testRunID].RemoveApplyFailSimulation(failID)

		Eventually(func(g Gomega) {
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
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
		failSet := dns.NewDNSSet(dns.DNSSetName{DNSName: newDNSName}, nil)
		failSet.UpdateGroup = testRunID
		failSet.Sets.AddRecord("A", e1.Spec.Targets[0], defaultTTL)
		failID := mock.TestMock[testRunID].AddApplyFailSimulation(firstZoneID, &provider.ChangeRequest{
			Action:   provider.R_CREATE, // create as DNSName is changed
			Type:     "A",
			Addition: failSet,
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
			return mock.TestMock[testRunID].GetApplyFailSimulationCount(failID)
		}).ShouldNot(BeZero())

		Eventually(func(g Gomega) {
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
			g.Expect(e1.Status.State).To(Equal("Stale"))
			g.Expect(e1.Status.ObservedGeneration).To(Equal(e1.Generation))
		}).Should(Succeed())

		mock.TestMock[testRunID].RemoveApplyFailSimulation(failID)

		Eventually(func(g Gomega) {
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e1), e1)).To(Succeed())
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
		deleteSet := dns.NewDNSSet(dns.DNSSetName{DNSName: "e2.first.example.com"}, nil)
		deleteSet.Sets.AddRecord("A", "1.1.2.1", 42)
		deleteSet.Sets.AddRecord("A", "1.1.2.2", 42)
		failID := mock.TestMock[testRunID].AddApplyFailSimulation(firstZoneID, &provider.ChangeRequest{
			Action:   provider.R_DELETE,
			Type:     "A",
			Deletion: deleteSet,
		})
		deleteSet2 := dns.NewDNSSet(dns.DNSSetName{DNSName: "e2.first.example.com"}, nil)
		deleteSet2.Sets.AddRecord("META", "\"owner=dnscontroller\"", 600)
		deleteSet2.Sets.AddRecord("META", "\"prefix=comment-\"", 600)
		failID2 := mock.TestMock[testRunID].AddApplyFailSimulation(firstZoneID, &provider.ChangeRequest{
			Action:   provider.R_DELETE,
			Type:     "META",
			Deletion: deleteSet2,
		})
		Expect(testClient.Delete(ctx, e2)).To(Succeed())

		Eventually(func() int {
			return mock.TestMock[testRunID].GetApplyFailSimulationCount(failID)
		}).ShouldNot(BeZero())
		Eventually(func() int {
			return mock.TestMock[testRunID].GetApplyFailSimulationCount(failID2)
		}).ShouldNot(BeZero())

		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(e2), e2)).To(Succeed())
		Expect(e2.DeletionTimestamp).NotTo(BeNil())

		// remove apply fail simulation
		mock.TestMock[testRunID].RemoveApplyFailSimulation(failID)
		mock.TestMock[testRunID].RemoveApplyFailSimulation(failID2)
		By("await deletion of entry " + e2.Name)
		Eventually(func(g Gomega) {
			checkDeleted(g, ctx, e2)
		}).WithTimeout(retryTimeout).Should(Succeed())

		By("check mock database")
		checkSingleEntryInMockDatabase(nil)
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
