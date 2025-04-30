// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package source_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/provider/mock"
)

var debug = false

var _ = Describe("DNSEntry source and DNSProvider replication controller tests", func() {
	const (
		entryCount = 5
	)

	var (
		testRunID      string
		testRunID2     string
		testNamespace1 *corev1.Namespace
		testNamespace2 *corev1.Namespace
		provider       *v1alpha1.DNSProvider
		entries        []*v1alpha1.DNSEntry

		checkMockDatabaseSize = func(expected int) {
			dump := mock.TestMock[testRunID].BuildFullDump()
			for _, zoneDump := range dump.InMemory {
				switch zoneDump.HostedZone.Domain {
				case "first.example.com":
					ExpectWithOffset(1, zoneDump.DNSSets).To(HaveLen(expected), "unexpected number of DNS sets in first.example.com")
				default:
					Fail("unexpected zone domain " + zoneDump.HostedZone.Domain)
				}
			}
		}

		checkTargetEntry = func(entry *v1alpha1.DNSEntry) {
			Eventually(func(g Gomega) {
				g.ExpectWithOffset(1, tc1.client.Get(ctx, client.ObjectKeyFromObject(entry), entry)).To(Succeed())
				g.ExpectWithOffset(1, entry.Status.State).To(Equal("Ready"))
				g.ExpectWithOffset(1, entry.Status.Targets).To(Equal(entry.Spec.Targets))
				g.ExpectWithOffset(1, entry.Status.ObservedGeneration).To(Equal(entry.Generation))

				list := &v1alpha1.DNSEntryList{}
				g.Expect(tc2.client.List(ctx, list, client.InNamespace(testRunID2))).To(Succeed())
				var targetEntry *v1alpha1.DNSEntry
				for _, e := range list.Items {
					if strings.HasPrefix(e.GetGenerateName(), fmt.Sprintf("%s-dnsentry", entry.Name)) {
						targetEntry = &e
						break
					}
				}
				g.Expect(targetEntry).NotTo(BeNil())
				g.Expect(targetEntry.Spec.DNSName).To(Equal(entry.Spec.DNSName))
				g.Expect(targetEntry.Spec.Targets).To(Equal(entry.Spec.Targets))
			}).Should(Succeed())
		}
	)

	BeforeEach(func() {
		SetDefaultEventuallyTimeout(10 * time.Second)
		if debug {
			SetDefaultEventuallyTimeout(30 * time.Second)
		}

		ctxLocal := context.Background()
		ctx0 := ctxutil.CancelContext(ctxutil.WaitGroupContext(context.Background(), "main"))
		ctx = ctxutil.TickContext(ctx0, controllermanager.DeletionActivity)

		By("Create test Namespace")
		testNamespace1 = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "source-",
			},
		}
		testNamespace2 = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "source-",
			},
		}
		Expect(tc1.client.Create(ctxLocal, testNamespace1)).To(Succeed())
		Expect(tc2.client.Create(ctxLocal, testNamespace2)).To(Succeed())
		log.Info("Created Namespace for test", "namespaceName", testNamespace1.Name)
		log.Info("Created Namespace for test in second cluster", "namespaceName", testNamespace2.Name)
		testRunID = testNamespace1.Name
		testRunID2 = testNamespace2.Name

		DeferCleanup(func() {
			By("Delete test Namespace")
			Expect(tc1.client.Delete(ctxLocal, testNamespace1)).To(Or(Succeed(), BeNotFoundError()))
			Expect(tc2.client.Delete(ctxLocal, testNamespace2)).To(Or(Succeed(), BeNotFoundError()))
		})

		By("Start manager")
		go func() {
			defer GinkgoRecover()
			args := []string{
				"--kubeconfig", tc1.kubeconfigFile,
				"--kubeconfig.id=source-id",
				"--target", tc2.kubeconfigFile,
				"--target.id=target-id",
				"--target.disable-deploy-crds",
				"--controllers", "compound,dnsentry-source,dnsprovider-replication",
				"--omit-lease",
				"--dns-delay", "1s",
				"--reschedule-delay", "2s",
				"--lock-status-check-period", "5s",
				"--pool.size", "2",
				"--dns-target-class=gardendns",
				"--dns-class=gardendns",
				"--target-namespace", testRunID2,
				"--disable-namespace-restriction",
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
			},
			LatencyMillis: 900,
		}
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
		Expect(tc1.client.Create(ctx, providerSecret)).To(Succeed())
		DeferCleanup(func() {
			Expect(tc1.client.Delete(ctx, providerSecret)).To(Succeed())
		})
		provider = &v1alpha1.DNSProvider{
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
		Expect(tc1.client.Create(ctx, provider)).To(Succeed())

		println("source kubeconfig:", tc1.kubeconfigFile)
		println("target kubeconfig:", tc2.kubeconfigFile)

		Eventually(func(g Gomega) {
			g.Expect(tc1.client.Get(ctx, client.ObjectKeyFromObject(provider), provider)).To(Succeed())
			g.Expect(provider.Status.State).To(Equal("Ready"))
		}).Should(Succeed())

		for i := 0; i < entryCount; i++ {
			name := fmt.Sprintf("e%d", i)
			if i == entryCount-1 {
				name += "-very-very-very-very-long-name-with-really-more-than-63-characters"
			}
			entries = append(entries, &v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testRunID,
					Name:      name,
				},
				Spec: v1alpha1.DNSEntrySpec{
					DNSName: fmt.Sprintf("e%d.first.example.com", i),
					Targets: []string{fmt.Sprintf("1.1.1.%d", i)},
				},
			})
		}
	})

	It("should create entries on target", func() {
		for _, entry := range entries {
			Expect(tc1.client.Create(ctx, entry)).To(Succeed())
		}

		By("check entries")
		for _, entry := range entries {
			checkTargetEntry(entry)
		}

		checkMockDatabaseSize(entryCount)

		By("check update")
		for _, entry := range entries {
			Expect(tc1.client.Get(ctx, client.ObjectKeyFromObject(entry), entry)).To(Succeed())
			entry.Spec.Targets = []string{fmt.Sprintf("2.2.2.%d", entryCount)}
			Expect(tc1.client.Update(ctx, entry)).To(Succeed())
		}

		By("check entries after update")
		for _, entry := range entries {
			checkTargetEntry(entry)
		}

		By("check recreation of target entries")
		list := &v1alpha1.DNSEntryList{}
		Expect(tc2.client.List(ctx, list, client.InNamespace(testRunID2))).To(Succeed())
		for _, entry := range list.Items {
			if strings.HasPrefix(entry.GetGenerateName(), fmt.Sprintf("%s-dnsentry", entry.Name)) {
				Expect(tc2.client.Delete(ctx, &entry)).To(Succeed())
			}
		}

		By("check new target entries")
		for _, entry := range entries {
			checkTargetEntry(entry)
		}

		By("delete entries")
		for _, entry := range entries {
			Expect(tc1.client.Delete(ctx, entry)).To(Succeed())
		}

		By("wait for deletion")
		for _, entry := range entries {
			for {
				if err := tc1.client.Get(ctx, client.ObjectKeyFromObject(entry), entry); err != nil {
					if client.IgnoreNotFound(err) == nil {
						break
					}
					Expect(err).NotTo(HaveOccurred())
				}
				time.Sleep(100 * time.Millisecond)
			}
		}

		Expect(tc1.client.Delete(ctx, provider)).To(Succeed())

		By("check mock database")
		checkMockDatabaseSize(0)
	})
})
