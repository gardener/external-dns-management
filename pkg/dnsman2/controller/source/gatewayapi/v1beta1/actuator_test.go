// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi/v1beta1"
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
		reconciler     *common.SourceReconciler[*gatewayapisv1beta1.Gateway]
		gateway        *gatewayapisv1beta1.Gateway
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
		gateway = &gatewayapisv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gateway",
				Namespace: defaultSourceNamespace,
				Annotations: map[string]string{
					"dns.gardener.cloud/dnsnames": "example.com",
				},
			},
			Spec: gatewayapisv1beta1.GatewaySpec{
				Listeners: []gatewayapisv1beta1.Listener{
					{Hostname: ptr.To(gatewayapisv1beta1.Hostname("example.com"))},
					{Hostname: ptr.To(gatewayapisv1beta1.Hostname("gardener.cloud"))},
					{Hostname: ptr.To(gatewayapisv1beta1.Hostname("wikipedia.org"))},
				},
			},
			Status: gatewayapisv1beta1.GatewayStatus{
				Addresses: []gatewayapisv1.GatewayStatusAddress{
					{Type: ptr.To(gatewayapisv1beta1.HostnameAddressType), Value: "example.com"},
					{Type: ptr.To(gatewayapisv1beta1.IPAddressType), Value: "1.1.1.1"},
				},
			},
		}
	})

	AfterEach(func() {
		close(fakeRecorder.Events)
	})

	Describe("#ReconcileSourceObject", func() {
		It("should create a DNSEntry", func() {
			Expect(fakeClientSrc.Create(ctx, gateway)).To(Succeed())

			err := doReconcile(ctx, reconciler, gateway)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))
			Expect(dnsEntries.Items[0].Spec.DNSName).To(Equal("example.com"))
			Expect(dnsEntries.Items[0].Spec.Targets).To(ContainElements("1.1.1.1"))
			testutils.AssertEvents(fakeRecorder.Events, "Normal DNSEntryCreated ")
		})

		It("should create a DNSEntry and clean it up when the Gateway is deleted", func() {
			Expect(fakeClientSrc.Create(ctx, gateway)).To(Succeed())

			err := doReconcile(ctx, reconciler, gateway)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))

			Expect(fakeClientSrc.Delete(ctx, gateway)).To(Succeed())

			err = doReconcile(ctx, reconciler, gateway)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries = getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(BeEmpty())
		})

		It("should handle the wildcard DNS names annotation", func() {
			gateway.Annotations["dns.gardener.cloud/dnsnames"] = "*"
			Expect(fakeClientSrc.Create(ctx, gateway)).To(Succeed())

			err := doReconcile(ctx, reconciler, gateway)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(3))
		})

		It("should create nothing without the DNS names annotation", func() {
			delete(gateway.Annotations, "dns.gardener.cloud/dnsnames")
			Expect(fakeClientSrc.Create(ctx, gateway)).To(Succeed())

			err := doReconcile(ctx, reconciler, gateway)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(BeEmpty())
		})

		It("should reject an empty DNS names annotation", func() {
			gateway.Annotations["dns.gardener.cloud/dnsnames"] = ""
			Expect(fakeClientSrc.Create(ctx, gateway)).To(Succeed())

			err := doReconcile(ctx, reconciler, gateway)
			Expect(err).To(MatchError("empty value for annotation \"dns.gardener.cloud/dnsnames\""))
		})

		It("should create nothing with the wrong DNS class annotation", func() {
			gateway.Annotations["dns.gardener.cloud/class"] = "other-class"
			Expect(fakeClientSrc.Create(ctx, gateway)).To(Succeed())

			err := doReconcile(ctx, reconciler, gateway)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(BeEmpty())
		})

		It("should reject a Gateway with a mismatch between declared and annotated DNS names", func() {
			gateway.Annotations["dns.gardener.cloud/dnsnames"] = "example.com,not-listened.to"
			Expect(fakeClientSrc.Create(ctx, gateway)).To(Succeed())

			err := doReconcile(ctx, reconciler, gateway)
			Expect(err).To(MatchError("annotated dns names not-listened.to not declared by gateway.spec.listeners[].hostname"))
		})

		It("should handle applying annotations from source to target", func() {
			gateway.Annotations["dns.gardener.cloud/resolve-targets-to-addresses"] = "true"
			gateway.Annotations["dns.gardener.cloud/cname-lookup-interval"] = "456"

			Expect(fakeClientSrc.Create(ctx, gateway)).To(Succeed())

			err := doReconcile(ctx, reconciler, gateway)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))
			Expect(dnsEntries.Items[0].Spec.ResolveTargetsToAddresses).To(Equal(ptr.To(true)))
			Expect(dnsEntries.Items[0].Spec.CNameLookupInterval).To(Equal(ptr.To(int64(456))))
		})

		It("should delete an obsolete DNSEntry when the Gateway is updated", func() {
			Expect(fakeClientSrc.Create(ctx, gateway)).To(Succeed())

			err := doReconcile(ctx, reconciler, gateway)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)).To(Succeed())
			delete(gateway.Annotations, "dns.gardener.cloud/dnsnames")
			Expect(fakeClientSrc.Update(ctx, gateway)).To(Succeed())

			err = doReconcile(ctx, reconciler, gateway)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries = getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(BeEmpty())
		})

		It("should drop an obsolete DNSEntry and create a new one when the Gateway is updated", func() {
			Expect(fakeClientSrc.Create(ctx, gateway)).To(Succeed())

			err := doReconcile(ctx, reconciler, gateway)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries := getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))

			Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)).To(Succeed())
			gateway.Annotations["dns.gardener.cloud/dnsnames"] = "gardener.cloud"
			Expect(fakeClientSrc.Update(ctx, gateway)).To(Succeed())

			err = doReconcile(ctx, reconciler, gateway)
			Expect(err).NotTo(HaveOccurred())

			dnsEntries = getDNSEntries(ctx, fakeClientCtrl, reconciler)
			Expect(dnsEntries.Items).To(HaveLen(1))
			Expect(dnsEntries.Items[0].Spec.DNSName).To(Equal("gardener.cloud"))
		})
	})
})

func doReconcile(ctx context.Context, reconciler *common.SourceReconciler[*gatewayapisv1beta1.Gateway], gateway *gatewayapisv1beta1.Gateway) error {
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: gateway.Namespace, Name: gateway.Name}}
	_, err := reconciler.Reconcile(ctx, req)
	return err
}

func getDNSEntries(ctx context.Context, c client.Client, reconciler *common.SourceReconciler[*gatewayapisv1beta1.Gateway]) *dnsv1alpha1.DNSEntryList {
	dnsEntries := &dnsv1alpha1.DNSEntryList{}
	ExpectWithOffset(1, c.List(ctx, dnsEntries, client.InNamespace(*reconciler.Config.TargetNamespace))).To(Succeed())
	return dnsEntries
}
