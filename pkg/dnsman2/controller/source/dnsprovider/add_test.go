package dnsprovider

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
)

var _ = Describe("Add", func() {
	Describe("#providersToReconcileOnSecretChanges", func() {
		var (
			ctx            = context.Background()
			fakeClientSrc  client.Client
			fakeClientCtrl client.Client
			reconciler     *Reconciler
		)

		BeforeEach(func() {
			fakeClientSrc = fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).WithStatusSubresource(&v1alpha1.DNSProvider{}).Build()
			fakeClientCtrl = fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).WithStatusSubresource(&v1alpha1.DNSProvider{}).Build()

			reconciler = &Reconciler{
				Client:             fakeClientSrc,
				ControlPlaneClient: fakeClientCtrl,
				Config: config.DNSManagerConfiguration{
					Controllers: config.ControllerConfiguration{
						DNSProvider: config.DNSProviderControllerConfig{
							Namespace: "test",
						},
					},
				},
			}

			Expect(fakeClientSrc.Create(ctx, &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider1", Namespace: "test"},
				Spec: v1alpha1.DNSProviderSpec{
					SecretRef: &corev1.SecretReference{
						Name: "secret1",
					},
				},
			})).To(Succeed())
			Expect(fakeClientSrc.Create(ctx, &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider2", Namespace: "test"},
				Spec: v1alpha1.DNSProviderSpec{
					SecretRef: &corev1.SecretReference{
						Name: "secret2",
					},
				},
			})).To(Succeed())
			Expect(fakeClientSrc.Create(ctx, &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider1b", Namespace: "test"},
				Spec: v1alpha1.DNSProviderSpec{
					SecretRef: &corev1.SecretReference{
						Name: "secret1",
					},
				},
			})).To(Succeed())
		})

		It("should find all matching providers", func() {
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret1", Namespace: "test"}}
			Expect(reconciler.providersToReconcileOnSecretChanges(ctx, "", secret)).To(ConsistOf(
				reconcile.Request{NamespacedName: client.ObjectKey{Name: "provider1", Namespace: "test"}},
				reconcile.Request{NamespacedName: client.ObjectKey{Name: "provider1b", Namespace: "test"}},
			))
		})

		It("should return empty list for unknown secret", func() {
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret42", Namespace: "test"}}
			Expect(reconciler.providersToReconcileOnSecretChanges(ctx, "", secret)).To(BeEmpty())
		})

	})

	Describe("#providersToReconcileOnProviderChanges", func() {
		var (
			reconciler *Reconciler
		)

		BeforeEach(func() {
			reconciler = &Reconciler{
				Config: config.DNSManagerConfiguration{
					Controllers: config.ControllerConfiguration{
						DNSProvider: config.DNSProviderControllerConfig{
							Namespace: "target-namespace",
						},
						Source: config.SourceControllerConfig{
							TargetNamespace: ptr.To("target-namespace"),
						},
					},
				},
				GVK: v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.DNSProviderKind),
			}
		})

		It("should return reconcile requests for valid DNSProvider on same cluster", func() {
			targetProvider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-provider",
					Namespace: "target-namespace",
					Annotations: map[string]string{
						"resources.gardener.cloud/owners": "dns.gardener.cloud/DNSProvider/test-namespace/owner1",
					},
				},
			}

			requests := reconciler.providersToReconcileOnProviderChanges(targetProvider)
			Expect(requests).To(Equal([]reconcile.Request{{NamespacedName: client.ObjectKey{Name: "owner1", Namespace: "test-namespace"}}}))
		})

		It("should return reconcile requests for valid DNSProvider on different cluster", func() {
			reconciler.Config.Controllers.Source.TargetClusterID = ptr.To("my-seed")
			reconciler.Config.Controllers.Source.SourceClusterID = ptr.To("other-cluster")
			targetProvider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-provider",
					Namespace: "target-namespace",
					Annotations: map[string]string{
						"resources.gardener.cloud/owners": "other-cluster:dns.gardener.cloud/DNSProvider/test-namespace/owner1",
					},
				},
			}

			requests := reconciler.providersToReconcileOnProviderChanges(targetProvider)
			Expect(requests).To(Equal([]reconcile.Request{{NamespacedName: client.ObjectKey{Name: "owner1", Namespace: "test-namespace"}}}))
		})

		It("should return no reconcile requests for foreign owners", func() {
			targetProvider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-provider",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						"resources.gardener.cloud/owners": "other-cluster:/Service/target-namespace/owner1",
					},
				},
			}

			requests := reconciler.providersToReconcileOnProviderChanges(targetProvider)
			Expect(requests).To(BeEmpty())
		})

		It("should return empty list if no annotation is set", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-secret",
					Namespace: "test-namespace",
				},
			}

			requests := reconciler.providersToReconcileOnProviderChanges(secret)
			Expect(requests).To(BeEmpty())
		})
	})

})
