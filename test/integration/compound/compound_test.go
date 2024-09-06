/*
 * // SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 * //
 * // SPDX-License-Identifier: Apache-2.0
 */

package compound_test

import (
	"context"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	var (
		testRunID     string
		testNamespace *corev1.Namespace
	)

	BeforeEach(func() {
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
				"--reschedule-delay", "15s",
				"--lock-status-check-period", "5s",
				"--pool.size", "10",
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
	})

	It("should create and update entries", func() {
		e1 := &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "e1",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "e1.first.example.com",
				Targets: []string{"1.1.1.1"},
			},
		}
		e2 := &v1alpha1.DNSEntry{
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
		e3 := &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "e3",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "e3.first.example.com",
				Text:    []string{"foo bar", "blabla"},
			},
		}
		e4 := &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testRunID,
				Name:      "e4",
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "e4.first.example.com",
				Targets: []string{"wikipedia.org"},
			},
		}
		for _, entry := range []*v1alpha1.DNSEntry{e1, e2, e3, e4} {
			Expect(testClient.Create(ctx, entry)).To(Succeed())
		}

		for _, entry := range []*v1alpha1.DNSEntry{e1, e2, e3, e4} {
			By("entry " + entry.Name)
			Eventually(func(g Gomega) {
				Expect(testClient.Get(ctx, client.ObjectKeyFromObject(entry), entry)).To(Succeed())
				g.Expect(entry.Status.State).To(Equal("Ready"))
				g.Expect(entry.Status.Targets).To(Equal(entry.Spec.Targets))
				g.Expect(entry.Status.Text).To(Equal(entry.Spec.Text))
				g.Expect(entry.Status.TTL).To(Equal(entry.Spec.TTL))
				g.Expect(entry.Status.ObservedGeneration).To(Equal(entry.Generation))
			}).Should(Succeed())
		}
	})
})
