// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ingress_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/ingress"
	"github.com/gardener/external-dns-management/pkg/dnsman2/testutils"
)

var _ = Describe("Actuator", func() {
	const (
		defaultTargetNamespace = "target-namespace"
		defaultSourceNamespace = "test"
	)
	var (
		ctx            = context.Background()
		fakeClientSrc  client.Client
		fakeClientCtrl client.Client
		fakeRecorder   *record.FakeRecorder
		ingress        *networkingv1.Ingress
		reconciler     *common.SourceReconciler[*networkingv1.Ingress]
	)

	BeforeEach(func() {
		fakeClientSrc = fakeclient.NewClientBuilder().WithScheme(dnsclient.ClusterScheme).WithStatusSubresource(&dnsv1alpha1.DNSAnnotation{}).Build()
		fakeClientCtrl = fakeclient.NewClientBuilder().WithScheme(dnsclient.ClusterScheme).Build()
		reconciler = common.NewSourceReconciler(&Actuator{})
		reconciler.Client = fakeClientSrc
		reconciler.ControlPlaneClient = fakeClientCtrl
		reconciler.Config = config.SourceControllerConfig{
			TargetNamespace: ptr.To(defaultTargetNamespace),
		}
		reconciler.State.Reset()
		fakeRecorder = record.NewFakeRecorder(32)
		reconciler.Recorder = common.NewDedupRecorder(fakeRecorder, 1*time.Second)
		ingress = &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ingress",
				Namespace: defaultSourceNamespace,
				Annotations: map[string]string{
					"dns.gardener.cloud/dnsnames": "example.com",
				},
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{Host: "example.com"},
					{Host: "gardener.cloud"},
					{Host: "wikipedia.org"},
				},
			},
			Status: networkingv1.IngressStatus{
				LoadBalancer: networkingv1.IngressLoadBalancerStatus{
					Ingress: []networkingv1.IngressLoadBalancerIngress{
						{Hostname: "example.com"},
						{IP: "1.1.1.1"},
					},
				},
			},
		}
	})

	AfterEach(func() {
		close(fakeRecorder.Events)
	})

	Describe("#ReconcileSourceObject", func() {
		It("should create a DNSEntry", func() {
			Expect(fakeClientSrc.Create(ctx, ingress)).To(Succeed())

			err := doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))
			Expect(dnsEntries.Items[0].Spec.DNSName).To(Equal("example.com"))
			Expect(dnsEntries.Items[0].Spec.Targets).To(ContainElements("1.1.1.1"))
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ")
		})

		It("should create a DNSEntry and clean it up when the Ingress is deleted", func() {
			Expect(fakeClientSrc.Create(ctx, ingress)).To(Succeed())

			err := doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))

			Expect(fakeClientSrc.Delete(ctx, ingress)).To(Succeed())

			err = doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries = getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(BeEmpty())
		})

		It("should handle the wildcard DNS names annotation", func() {
			ingress.Annotations["dns.gardener.cloud/dnsnames"] = "*"
			Expect(fakeClientSrc.Create(ctx, ingress)).To(Succeed())

			err := doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(3))

		})

		It("should create nothing without the DNS names annotation", func() {
			delete(ingress.Annotations, "dns.gardener.cloud/dnsnames")
			Expect(fakeClientSrc.Create(ctx, ingress)).To(Succeed())

			err := doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(BeEmpty())
		})

		It("should reject an empty DNS names annotation", func() {
			ingress.Annotations["dns.gardener.cloud/dnsnames"] = ""
			Expect(fakeClientSrc.Create(ctx, ingress)).To(Succeed())

			err := doReconcile(ctx, reconciler, ingress)
			Expect(err).To(MatchError("empty value for annotation \"dns.gardener.cloud/dnsnames\""))
		})

		It("should create nothing with the wrong DNS class annotation", func() {
			ingress.Annotations["dns.gardener.cloud/class"] = "other-class"
			Expect(fakeClientSrc.Create(ctx, ingress)).To(Succeed())

			err := doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(BeEmpty())
		})

		It("should reject an Ingress with a mismatch between declared and annotated DNS names", func() {
			ingress.Annotations["dns.gardener.cloud/dnsnames"] = "example.com,not-declared.com"
			Expect(fakeClientSrc.Create(ctx, ingress)).To(Succeed())

			err := doReconcile(ctx, reconciler, ingress)
			Expect(err).To(MatchError("annotated dns names not-declared.com not declared by ingress"))
		})

		It("should handle the resolve targets to addresses annotation", func() {
			ingress.Annotations["dns.gardener.cloud/resolve-targets-to-addresses"] = "true"
			ingress.Annotations["dns.gardener.cloud/cname-lookup-interval"] = "456"

			Expect(fakeClientSrc.Create(ctx, ingress)).To(Succeed())

			err := doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))
			Expect(dnsEntries.Items[0].Spec.ResolveTargetsToAddresses).To(Equal(ptr.To(true)))
			Expect(dnsEntries.Items[0].Spec.CNameLookupInterval).To(Equal(ptr.To(int64(456))))
		})

		It("should handle the IP stack annotation", func() {
			ingress.Annotations["dns.gardener.cloud/ip-stack"] = "ipv6"
			Expect(fakeClientSrc.Create(ctx, ingress)).To(Succeed())

			err := doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))
			Expect(dnsEntries.Items[0].Annotations["dns.gardener.cloud/ip-stack"]).To(Equal("ipv6"))
		})

		It("should delete an obsolete DNSEntry when the Ingress is updated", func() {
			Expect(fakeClientSrc.Create(ctx, ingress)).To(Succeed())

			err := doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(ingress), ingress)).To(Succeed())
			delete(ingress.Annotations, "dns.gardener.cloud/dnsnames")
			Expect(fakeClientSrc.Update(ctx, ingress)).To(Succeed())

			err = doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries = getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(BeEmpty())
		})

		It("should drop an obsolete DNSEntry and create a new one when the Ingress is updated", func() {
			Expect(fakeClientSrc.Create(ctx, ingress)).To(Succeed())

			err := doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(ingress), ingress)).To(Succeed())
			ingress.Annotations["dns.gardener.cloud/dnsnames"] = "gardener.cloud"
			Expect(fakeClientSrc.Update(ctx, ingress)).To(Succeed())

			err = doReconcile(ctx, reconciler, ingress)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries = getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))
			Expect(dnsEntries.Items[0].Spec.DNSName).To(Equal("gardener.cloud"))
		})
	})
})

func doReconcile(ctx context.Context, reconciler *common.SourceReconciler[*networkingv1.Ingress], ingress *networkingv1.Ingress) error {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name}}
	_, err := reconciler.Reconcile(ctx, req)
	return err
}

func getDNSEntries(ctx context.Context, c client.Client, reconciler *common.SourceReconciler[*networkingv1.Ingress]) *dnsv1alpha1.DNSEntryList {
	dnsEntries := &dnsv1alpha1.DNSEntryList{}
	ExpectWithOffset(1, c.List(ctx, dnsEntries, client.InNamespace(*reconciler.Config.TargetNamespace))).To(Succeed())
	return dnsEntries
}
