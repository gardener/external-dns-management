package controlplane

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			ctx        = context.Background()
			fakeClient client.Client
			reconciler *Reconciler
		)

		BeforeEach(func() {
			fakeClient = fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).Build()
			reconciler = &Reconciler{
				Client: fakeClient,
				Config: config.DNSManagerConfiguration{
					Controllers: config.ControllerConfiguration{
						DNSProvider: config.DNSProviderControllerConfig{
							Namespace: "test",
						},
					},
				},
			}

			Expect(fakeClient.Create(ctx, &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider1", Namespace: "test"},
				Spec: v1alpha1.DNSProviderSpec{
					SecretRef: &corev1.SecretReference{
						Name: "secret1",
					},
				},
			})).To(Succeed())
			Expect(fakeClient.Create(ctx, &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "provider1b",
					Namespace: "test",
					Annotations: map[string]string{
						"dns.gardener.cloud/class": "foo",
					},
				},
				Spec: v1alpha1.DNSProviderSpec{
					SecretRef: &corev1.SecretReference{
						Name: "secret1",
					},
				},
			})).To(Succeed())
			Expect(fakeClient.Create(ctx, &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider2", Namespace: "test"},
				Spec: v1alpha1.DNSProviderSpec{
					SecretRef: &corev1.SecretReference{
						Name:      "secret1",
						Namespace: "test",
					},
				},
			})).To(Succeed())
			Expect(fakeClient.Create(ctx, &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider3", Namespace: "test"},
				Spec: v1alpha1.DNSProviderSpec{
					SecretRef: &corev1.SecretReference{
						Name: "secret3",
					},
				},
			})).To(Succeed())
		})

		It("should find all matching providers", func() {
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret1", Namespace: "test"}}
			Expect(reconciler.providersToReconcileOnSecretChanges(ctx, secret)).To(ConsistOf(
				reconcile.Request{NamespacedName: client.ObjectKey{Name: "provider1", Namespace: "test"}},
				reconcile.Request{NamespacedName: client.ObjectKey{Name: "provider2", Namespace: "test"}},
			))
		})

		It("should return empty list for unknown secret", func() {
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret42", Namespace: "test"}}
			Expect(reconciler.providersToReconcileOnSecretChanges(ctx, secret)).To(BeEmpty())
		})

	})
})
