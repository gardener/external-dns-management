// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/service"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("#CleanupOrphanOwnedDNSEntries", func() {
	const (
		targetNamespace = "target-namespace"
		sourceNamespace = "test"
	)

	var (
		ctx            = context.Background()
		fakeClientSrc  client.Client
		fakeClientCtrl client.Client
		fakeRecorder   *events.FakeRecorder
		reconciler     *common.SourceReconciler[*corev1.Service]

		// createServiceWithEntry creates a source Service with a load balancer target and reconciles
		// it once so that a replicated target DNSEntry owned by it exists on the control plane. It
		// returns the created service and its (single) owned target DNSEntry.
		createServiceWithEntry = func(name, dnsName string) (*corev1.Service, *dnsv1alpha1.DNSEntry) {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Namespace:   sourceNamespace,
					Annotations: map[string]string{dns.AnnotationDNSNames: dnsName},
				},
				Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
			}
			ExpectWithOffset(1, fakeClientSrc.Create(ctx, svc)).To(Succeed())
			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "1.2.3.4"}}
			ExpectWithOffset(1, fakeClientSrc.SubResource("Status").Update(ctx, svc)).To(Succeed())

			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}}
			_, err := reconciler.Reconcile(ctx, req)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			entries := listOwnedEntries(ctx, fakeClientCtrl, reconciler, svc)
			ExpectWithOffset(1, entries).To(HaveLen(1), "expected exactly one replicated target DNSEntry")
			return svc, entries[0]
		}

		// removeSource deletes the source Service and clears its finalizer so the fake client removes
		// the object, simulating a delete event that was missed while the controller was down.
		removeSource = func(svc *corev1.Service) {
			ExpectWithOffset(1, fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(svc), svc)).To(Succeed())
			svc.Finalizers = nil
			ExpectWithOffset(1, fakeClientSrc.Update(ctx, svc)).To(Succeed())
			ExpectWithOffset(1, fakeClientSrc.Delete(ctx, svc)).To(Succeed())
			ExpectWithOffset(1, errors.IsNotFound(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(svc), svc))).To(BeTrue())
		}
	)

	BeforeEach(func() {
		fakeClientSrc = fakeclient.NewClientBuilder().WithScheme(dnsclient.ClusterScheme).WithStatusSubresource(&corev1.Service{}, &dnsv1alpha1.DNSAnnotation{}).Build()
		fakeClientCtrl = fakeclient.NewClientBuilder().WithScheme(dnsclient.ClusterScheme).Build()
		reconciler = common.NewSourceReconciler(&Actuator{})
		reconciler.Client = fakeClientSrc
		reconciler.ControlPlaneClient = fakeClientCtrl
		reconciler.Config = config.SourceControllerConfig{
			TargetNamespace: ptr.To(targetNamespace),
		}
		reconciler.FinalizerName = dns.ClassSourceFinalizer(dns.NormalizeClass(""), "service-dns")
		reconciler.State.Reset()
		fakeRecorder = events.NewFakeRecorder(32)
		reconciler.Recorder = common.NewDedupRecorder(fakeRecorder, 1*time.Second)
	})

	AfterEach(func() {
		close(fakeRecorder.Events)
	})

	It("should delete the target DNSEntry of a source that no longer exists", func() {
		svc, entry := createServiceWithEntry("foo", "foo.example.com")

		By("removing the source Service without reconciling the delete (simulating a missed delete event)")
		removeSource(svc)

		By("running the orphan cleanup sweep")
		Expect(reconciler.CleanupOrphanOwnedDNSEntries(ctx)).To(Succeed())

		By("checking that the orphan target DNSEntry is deleted")
		err := fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(entry), &dnsv1alpha1.DNSEntry{})
		Expect(errors.IsNotFound(err)).To(BeTrue(), "orphan target DNSEntry was not deleted")
	})

	It("should keep the target DNSEntry of a source that still exists", func() {
		_, entry := createServiceWithEntry("foo", "foo.example.com")

		By("running the orphan cleanup sweep")
		Expect(reconciler.CleanupOrphanOwnedDNSEntries(ctx)).To(Succeed())

		By("checking that the target DNSEntry of the live source is kept")
		Expect(fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(entry), &dnsv1alpha1.DNSEntry{})).To(Succeed(), "target DNSEntry of a live source must not be deleted")
	})

	It("should only delete entries of missing sources when both orphan and live sources exist", func() {
		liveSvc, liveEntry := createServiceWithEntry("live", "live.example.com")
		orphanSvc, orphanEntry := createServiceWithEntry("orphan", "orphan.example.com")

		By("removing the orphan source Service")
		removeSource(orphanSvc)

		By("running the orphan cleanup sweep")
		Expect(reconciler.CleanupOrphanOwnedDNSEntries(ctx)).To(Succeed())

		By("checking that only the orphan target DNSEntry is deleted")
		Expect(errors.IsNotFound(fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(orphanEntry), &dnsv1alpha1.DNSEntry{}))).To(BeTrue(), "orphan target DNSEntry was not deleted")
		Expect(fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(liveEntry), &dnsv1alpha1.DNSEntry{})).To(Succeed(), "target DNSEntry of a live source must not be deleted")
		_ = liveSvc
	})

	It("should delete all target DNSEntries of a missing source that owns several", func() {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "foo",
				Namespace:   sourceNamespace,
				Annotations: map[string]string{dns.AnnotationDNSNames: "foo.example.com,bar.example.com"},
			},
			Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
		}
		Expect(fakeClientSrc.Create(ctx, svc)).To(Succeed())
		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "1.2.3.4"}}
		Expect(fakeClientSrc.SubResource("Status").Update(ctx, svc)).To(Succeed())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}}
		_, err := reconciler.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		entries := listOwnedEntries(ctx, fakeClientCtrl, reconciler, svc)
		Expect(entries).To(HaveLen(2), "expected two replicated target DNSEntries for a multi-name source")

		By("removing the source Service")
		removeSource(svc)

		By("running the orphan cleanup sweep")
		Expect(reconciler.CleanupOrphanOwnedDNSEntries(ctx)).To(Succeed())

		By("checking that all orphan target DNSEntries are deleted")
		for _, entry := range entries {
			err := fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(entry), &dnsv1alpha1.DNSEntry{})
			Expect(errors.IsNotFound(err)).To(BeTrue(), "orphan target DNSEntry %s was not deleted", entry.Name)
		}
	})

	It("should be a no-op when there are no target DNSEntries", func() {
		Expect(reconciler.CleanupOrphanOwnedDNSEntries(ctx)).To(Succeed())

		list := &dnsv1alpha1.DNSEntryList{}
		Expect(fakeClientCtrl.List(ctx, list, client.InNamespace(targetNamespace))).To(Succeed())
		Expect(list.Items).To(BeEmpty())
	})
})

// listOwnedEntries returns the replicated target DNSEntries owned by the given source service.
func listOwnedEntries(ctx context.Context, c client.Client, r *common.SourceReconciler[*corev1.Service], svc *corev1.Service) []*dnsv1alpha1.DNSEntry {
	list := &dnsv1alpha1.DNSEntryList{}
	ExpectWithOffset(1, c.List(ctx, list, client.InNamespace(*r.Config.TargetNamespace))).To(Succeed())
	ownerData := common.OwnerData{
		Object:    svc,
		GVK:       r.GVK,
		ClusterID: ptr.Deref(r.Config.SourceClusterID, ""),
	}
	var items []*dnsv1alpha1.DNSEntry
	for i := range list.Items {
		if ownerData.HasOwner(&list.Items[i], ptr.Deref(r.Config.TargetClusterID, "")) {
			items = append(items, &list.Items[i])
		}
	}
	return items
}
