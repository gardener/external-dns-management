package dnsentry

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

var _ = Describe("Add", func() {
	Describe("#entriesToReconcileOnProviderChanges", func() {
		var (
			ctx                    = context.Background()
			fakeClient             client.Client
			reconciler             *Reconciler
			key1, key2, key3, key4 client.ObjectKey

			checkEntriesToReconcileOnProviderChanges = func(ctx context.Context, provider client.Object) []reconcile.Request {
				GinkgoHelper()
				return reconciler.entriesToReconcileOnProviderChanges(ctx, logr.Discard(), provider)
			}
		)

		BeforeEach(func() {
			fakeClient = fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).Build()
			reconciler = &Reconciler{
				Client:    fakeClient,
				Namespace: "test",
				state:     state.GetState(),
			}
			reconciler.setCachePeriods(1*time.Microsecond, defaultDriftCheckPeriod, defaultProviderUpdateCachePeriod)

			Expect(fakeClient.Create(ctx, &v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry1", Namespace: "test"},
				Spec: v1alpha1.DNSEntrySpec{
					DNSName: "*.foo.example.com",
				},
			})).To(Succeed())
			key1 = client.ObjectKey{Name: "entry1", Namespace: "test"}
			Expect(fakeClient.Create(ctx, &v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry2", Namespace: "test"},
				Spec: v1alpha1.DNSEntrySpec{
					DNSName: "bar.example.com",
				},
			})).To(Succeed())
			key2 = client.ObjectKey{Name: "entry2", Namespace: "test"}
			Expect(fakeClient.Create(ctx, &v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry3", Namespace: "test"},
				Spec: v1alpha1.DNSEntrySpec{
					DNSName: "sub.bar.example.com",
				},
				Status: v1alpha1.DNSEntryStatus{
					State: v1alpha1.StateError,
				},
			})).To(Succeed())
			key3 = client.ObjectKey{Name: "entry3", Namespace: "test"}
			Expect(fakeClient.Create(ctx, &v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry4", Namespace: "test"},
				Spec: v1alpha1.DNSEntrySpec{
					DNSName: "*.acme.com",
				},
				Status: v1alpha1.DNSEntryStatus{
					State:    v1alpha1.StateReady,
					Provider: new("test/provider1"),
				},
			})).To(Succeed())
			key4 = client.ObjectKey{Name: "entry4", Namespace: "test"}
		})

		It("should find all matching entries", func() {
			provider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider1", Namespace: "test"},
				Status: v1alpha1.DNSProviderStatus{
					State: v1alpha1.StateReady,
					Domains: v1alpha1.DNSSelectionStatus{
						Included: []string{"example.com"},
					},
					LastUpdateTime: &metav1.Time{Time: time.Now()},
				},
			}
			expectedRequests := []any{
				reconcile.Request{NamespacedName: key1},
				reconcile.Request{NamespacedName: key2},
				reconcile.Request{NamespacedName: key3},
				reconcile.Request{NamespacedName: key4}, // matches because it is already assigned to the provider
			}
			Expect(checkEntriesToReconcileOnProviderChanges(ctx, provider)).To(ConsistOf(expectedRequests...))

			// second call should return empty list because unchanged
			requests := reconciler.entriesToReconcileOnProviderChanges(ctx, logr.Discard(), provider)
			Expect(requests).To(BeEmpty())

			// after updating LastUpdateTime expected requests should be returned
			provider.Status.LastUpdateTime.Time = provider.Status.LastUpdateTime.Add(1 * time.Microsecond)
			Expect(checkEntriesToReconcileOnProviderChanges(ctx, provider)).To(ConsistOf(expectedRequests...))
		})

		It("should skip fan-out if provider status has not caught up with spec", func() {
			provider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider1", Namespace: "test", Generation: 2},
				Status: v1alpha1.DNSProviderStatus{
					ObservedGeneration: 1,
					State:              v1alpha1.StateReady,
					Domains: v1alpha1.DNSSelectionStatus{
						Included: []string{"example.com"},
					},
					LastUpdateTime: &metav1.Time{Time: time.Now()},
				},
			}
			Expect(checkEntriesToReconcileOnProviderChanges(ctx, provider)).To(BeEmpty())

			// once status catches up, the fan-out should happen
			provider.Status.ObservedGeneration = 2
			Expect(checkEntriesToReconcileOnProviderChanges(ctx, provider)).To(ConsistOf(
				reconcile.Request{NamespacedName: key1},
				reconcile.Request{NamespacedName: key2},
				reconcile.Request{NamespacedName: key3},
				reconcile.Request{NamespacedName: key4},
			))
		})

		It("should return empty list for not matching provider", func() {
			provider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "other-provider", Namespace: "test"},
				Status: v1alpha1.DNSProviderStatus{
					State: v1alpha1.StateReady,
					Domains: v1alpha1.DNSSelectionStatus{
						Included: []string{"other-example.com"},
					},
				},
			}
			Expect(checkEntriesToReconcileOnProviderChanges(ctx, provider)).To(BeEmpty())
		})

		It("should return exact matching domain", func() {
			provider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider2", Namespace: "test"},
				Status: v1alpha1.DNSProviderStatus{
					State: v1alpha1.StateReady,
					Domains: v1alpha1.DNSSelectionStatus{
						Included: []string{"bar.example.com"},
						Excluded: []string{"sub.sub.bar.example.com"},
					},
				},
			}
			Expect(checkEntriesToReconcileOnProviderChanges(ctx, provider)).To(ConsistOf(
				reconcile.Request{NamespacedName: key2},
				reconcile.Request{NamespacedName: key3},
			))
		})

		It("should consider excluded domains correctly", func() {
			provider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider2", Namespace: "test"},
				Status: v1alpha1.DNSProviderStatus{
					State: v1alpha1.StateReady,
					Domains: v1alpha1.DNSSelectionStatus{
						Included: []string{"example.com"},
						Excluded: []string{"foo.example.com", "sub.bar.example.com"},
					},
				},
			}
			Expect(checkEntriesToReconcileOnProviderChanges(ctx, provider)).To(ConsistOf(
				reconcile.Request{NamespacedName: key2},
			))
		})

		It("should select by domain if state != ready (matching)", func() {
			provider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider2", Namespace: "test"},
				Status: v1alpha1.DNSProviderStatus{
					State: v1alpha1.StateReady,
					Domains: v1alpha1.DNSSelectionStatus{
						Included: []string{"sub.bar.example.com"},
					},
				},
			}
			Expect(checkEntriesToReconcileOnProviderChanges(ctx, provider)).To(ConsistOf(
				reconcile.Request{NamespacedName: key3},
			))
		})

		It("should select by domain if state != ready (non-matching)", func() {
			provider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{Name: "provider2", Namespace: "test"},
				Status: v1alpha1.DNSProviderStatus{
					State: v1alpha1.StateReady,
					Domains: v1alpha1.DNSSelectionStatus{
						Included: []string{"bla.bar.example.com"},
					},
				},
			}
			Expect(checkEntriesToReconcileOnProviderChanges(ctx, provider)).To(BeEmpty())
		})

	})
})
