// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package dnsprovider

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/local"
)

var _ = Describe("#cleanupOrphanTargetProviders", func() {
	const (
		targetNamespace = "target-namespace"
		sourceNamespace = "test"
	)

	var (
		ctx            = context.Background()
		fakeClientSrc  client.Client
		fakeClientCtrl client.Client
		fakeRecorder   *events.FakeRecorder
		reconciler     *Reconciler

		// createSourceProviderWithTarget creates a source DNSProvider plus its secret and reconciles
		// it once, so that a replicated target DNSProvider owned by it exists on the control plane.
		// It returns the created source provider and its (single) target provider.
		createSourceProviderWithTarget = func(name string) (*v1alpha1.DNSProvider, *v1alpha1.DNSProvider) {
			sourceSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name + "-secret", Namespace: sourceNamespace},
				Data:       map[string][]byte{"key": []byte("value")},
			}
			sourceProvider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: sourceNamespace},
				Spec: v1alpha1.DNSProviderSpec{
					Type:      local.ProviderType,
					SecretRef: &corev1.SecretReference{Name: sourceSecret.Name},
				},
			}
			ExpectWithOffset(1, fakeClientSrc.Create(ctx, sourceSecret)).To(Succeed())
			ExpectWithOffset(1, fakeClientSrc.Create(ctx, sourceProvider)).To(Succeed())

			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: sourceProvider.Namespace, Name: sourceProvider.Name}}
			_, err := reconciler.Reconcile(ctx, req)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			targets := listTargetProviders(ctx, fakeClientCtrl, reconciler, sourceProvider)
			ExpectWithOffset(1, targets).To(HaveLen(1), "expected exactly one replicated target DNSProvider")
			return sourceProvider, targets[0]
		}
	)

	BeforeEach(func() {
		fakeClientSrc = fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).WithStatusSubresource(&v1alpha1.DNSProvider{}).Build()
		fakeClientCtrl = fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).WithStatusSubresource(&v1alpha1.DNSProvider{}).Build()
		fakeRecorder = events.NewFakeRecorder(32)
		registry := provider.NewDNSHandlerRegistry(clock.RealClock{})
		local.RegisterTo(registry)
		reconciler = &Reconciler{
			Clock:              clock.RealClock{},
			Client:             fakeClientSrc,
			ControlPlaneClient: fakeClientCtrl,
			Config: config.SourceControllerConfig{
				TargetNamespace: ptr.To(targetNamespace),
			},
			GVK:               v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.DNSProviderKind),
			DNSHandlerFactory: registry,
			Recorder:          fakeRecorder,
		}
	})

	AfterEach(func() {
		close(fakeRecorder.Events)
	})

	It("should delete the target DNSProvider of a source that no longer exists", func() {
		sourceProvider, targetProvider := createSourceProviderWithTarget("foo")

		By("deleting the source DNSProvider without reconciling the delete (simulating a missed delete event)")
		Expect(fakeClientSrc.Delete(ctx, sourceProvider)).To(Succeed())
		// Clear the replication finalizer directly so the fake client actually removes the object.
		Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(sourceProvider), sourceProvider)).To(Succeed())
		sourceProvider.Finalizers = nil
		Expect(fakeClientSrc.Update(ctx, sourceProvider)).To(Succeed())
		Expect(errors.IsNotFound(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(sourceProvider), sourceProvider))).To(BeTrue())

		By("running the orphan cleanup sweep")
		Expect(reconciler.cleanupOrphanTargetProviders(ctx)).To(Succeed())

		By("checking that the orphan target DNSProvider is deleted")
		err := fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(targetProvider), &v1alpha1.DNSProvider{})
		Expect(errors.IsNotFound(err)).To(BeTrue(), "orphan target DNSProvider was not deleted")
	})

	It("should keep the target DNSProvider of a source that still exists", func() {
		_, targetProvider := createSourceProviderWithTarget("foo")

		By("running the orphan cleanup sweep")
		Expect(reconciler.cleanupOrphanTargetProviders(ctx)).To(Succeed())

		By("checking that the target DNSProvider of the live source is kept")
		Expect(fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(targetProvider), &v1alpha1.DNSProvider{})).To(Succeed(), "target DNSProvider of a live source must not be deleted")
	})

	It("should only delete targets of missing sources when both orphan and live sources exist", func() {
		liveSource, liveTarget := createSourceProviderWithTarget("live")
		orphanSource, orphanTarget := createSourceProviderWithTarget("orphan")

		By("removing the orphan source object")
		Expect(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(orphanSource), orphanSource)).To(Succeed())
		orphanSource.Finalizers = nil
		Expect(fakeClientSrc.Update(ctx, orphanSource)).To(Succeed())
		Expect(fakeClientSrc.Delete(ctx, orphanSource)).To(Succeed())
		Expect(errors.IsNotFound(fakeClientSrc.Get(ctx, client.ObjectKeyFromObject(orphanSource), orphanSource))).To(BeTrue())

		By("running the orphan cleanup sweep")
		Expect(reconciler.cleanupOrphanTargetProviders(ctx)).To(Succeed())

		By("checking that only the orphan target DNSProvider is deleted")
		Expect(errors.IsNotFound(fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(orphanTarget), &v1alpha1.DNSProvider{}))).To(BeTrue(), "orphan target DNSProvider was not deleted")
		Expect(fakeClientCtrl.Get(ctx, client.ObjectKeyFromObject(liveTarget), &v1alpha1.DNSProvider{})).To(Succeed(), "target DNSProvider of a live source must not be deleted")
		_ = liveSource
	})

	It("should be a no-op when there are no target DNSProviders", func() {
		Expect(reconciler.cleanupOrphanTargetProviders(ctx)).To(Succeed())

		list := &v1alpha1.DNSProviderList{}
		Expect(fakeClientCtrl.List(ctx, list, client.InNamespace(targetNamespace))).To(Succeed())
		Expect(list.Items).To(BeEmpty())
	})
})

// listTargetProviders returns the replicated target DNSProviders owned by the given source provider.
func listTargetProviders(ctx context.Context, c client.Client, r *Reconciler, sourceProvider *v1alpha1.DNSProvider) []*v1alpha1.DNSProvider {
	list := &v1alpha1.DNSProviderList{}
	ExpectWithOffset(1, c.List(ctx, list, client.InNamespace(*r.Config.TargetNamespace))).To(Succeed())
	ownerData := common.OwnerData{
		Object:    sourceProvider,
		GVK:       r.GVK,
		ClusterID: ptr.Deref(r.Config.SourceClusterID, ""),
	}
	var items []*v1alpha1.DNSProvider
	for i := range list.Items {
		if ownerData.HasOwner(&list.Items[i], ptr.Deref(r.Config.TargetClusterID, "")) {
			items = append(items, &list.Items[i])
		}
	}
	return items
}
