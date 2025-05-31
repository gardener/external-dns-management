package dnsentry

import (
	"context"
	"time"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/selection"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
)

type lightDNSHostedZone struct {
	id     dns.ZoneID
	domain string
}

func (z *lightDNSHostedZone) ZoneID() dns.ZoneID { return z.id }
func (z *lightDNSHostedZone) Domain() string     { return z.domain }

var _ = Describe("Reconcile", func() {
	var (
		ctx        = context.Background()
		fakeClient client.Client
		reconciler *Reconciler
		entryKey   = client.ObjectKey{Name: "entry1", Namespace: "test"}
		entry      *v1alpha1.DNSEntry
		clock      = &testing.FakeClock{}
		startTime  = time.Now().Truncate(time.Second)
		zones      = []selection.LightDNSHostedZone{
			&lightDNSHostedZone{id: dns.NewZoneID("mock", "z-example.com"), domain: "example.com"},
			&lightDNSHostedZone{id: dns.NewZoneID("mock", "z-example2.com"), domain: "example2.com"},
		}
		skipProviderState bool
		createProvider    = func(name string, included, excluded []string, providerState string) {
			provider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "test",
				},
				Spec: v1alpha1.DNSProviderSpec{
					Type:    "mock",
					Domains: &v1alpha1.DNSSelection{Include: included, Exclude: excluded},
				},
				Status: v1alpha1.DNSProviderStatus{
					State:              providerState,
					LastUpdateTime:     &metav1.Time{Time: startTime},
					ObservedGeneration: 1,
					Domains:            v1alpha1.DNSSelectionStatus{Included: included, Excluded: excluded},
				},
			}
			if !skipProviderState {
				providerState := state.GetState().GetOrCreateProviderState(provider, config.DNSProviderControllerConfig{})
				selectionResult := selection.CalcZoneAndDomainSelection(provider.Spec, zones)
				selectionResult.SetProviderStatusZonesAndDomains(&provider.Status)
				providerState.SetSelection(selectionResult)
			}
			ExpectWithOffset(1, fakeClient.Create(ctx, provider)).To(Succeed())
		}
		deleteProvider = func(name string) {
			provider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "test",
				},
			}
			ExpectWithOffset(1, fakeClient.Delete(ctx, provider)).To(Succeed())
			state.GetState().DeleteProviderState(client.ObjectKeyFromObject(provider))
		}
		updateProvider = func(name string, included, excluded []string, state string) {
			deleteProvider(name)
			createProvider(name, included, excluded, state)
		}
		checkEntryStatusProvider = func(providerName, providerType, zoneID string) {
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: entryKey})
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			ExpectWithOffset(1, result).To(Equal(reconcile.Result{}))
			ExpectWithOffset(1, fakeClient.Get(ctx, entryKey, entry)).To(Succeed())
			var ptrName, ptrType, ptrZoneID *string
			if providerName != "" {
				ptrName = ptr.To(providerName)
			}
			if providerType != "" {
				ptrType = ptr.To(providerType)
			}
			if zoneID != "" {
				ptrZoneID = ptr.To(zoneID)
			}
			ExpectWithOffset(1, entry.Status.Provider).To(Equal(ptrName))
			ExpectWithOffset(1, entry.Status.ProviderType).To(Equal(ptrType))
			ExpectWithOffset(1, entry.Status.Zone).To(Equal(ptrZoneID))
			ExpectWithOffset(1, entry.Status.ObservedGeneration).To(Equal(entry.Generation))

			if len(entry.Status.Targets) == 0 {
				ExpectWithOffset(1, entry.Finalizers).To(BeEmpty())
			} else {
				ExpectWithOffset(1, entry.Finalizers).To(Equal([]string{"dns.gardener.cloud/compound"}))
			}
		}
	)

	BeforeEach(func() {
		state.ClearState()
		clock.SetTime(startTime)
		fakeClient = fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).WithStatusSubresource(&v1alpha1.DNSEntry{}).Build()
		reconciler = &Reconciler{
			Client:    fakeClient,
			Namespace: "test",
			Clock:     clock,
			state:     state.GetState(),
		}

		Expect(fakeClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}})).To(Succeed())

		entry = &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:       entryKey.Name,
				Namespace:  entryKey.Namespace,
				Generation: 1,
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "test.sub.example.com",
			},
		}
	})

	It("should assign correct provider", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady)
		Expect(fakeClient.Create(ctx, entry)).To(Succeed())

		checkEntryStatusProvider("p1", "mock", "z-example.com")
	})

	It("should assign best matching provider", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady)
		createProvider("p3", []string{"sub.example.com"}, nil, v1alpha1.StateReady)
		Expect(fakeClient.Create(ctx, entry)).To(Succeed())

		checkEntryStatusProvider("p3", "mock", "z-example.com")

		By("should reassign second best provider after removing the best one")
		deleteProvider("p3")
		checkEntryStatusProvider("p1", "mock", "z-example.com")

		By("should reassign better provider")
		createProvider("p3", []string{"sub.example.com"}, nil, v1alpha1.StateReady)
		checkEntryStatusProvider("p3", "mock", "z-example.com")

		By("should reassign after old provider excludes domain")
		updateProvider("p3", []string{"sub.example.com"}, []string{"test.sub.example.com"}, v1alpha1.StateReady)
		checkEntryStatusProvider("p1", "mock", "z-example.com")
	})

	It("should keep old provider if provider changes state and no other provider is matching", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady)
		Expect(fakeClient.Create(ctx, entry)).To(Succeed())

		checkEntryStatusProvider("p1", "mock", "z-example.com")

		By("should keep old provider if it changes state to error")
		updateProvider("p1", []string{"example.com"}, nil, v1alpha1.StateError)
		checkEntryStatusProvider("p1", "mock", "z-example.com")

		By("but should change to new matching, ready provider")
		createProvider("p3", []string{"example.com"}, nil, v1alpha1.StateReady)
		checkEntryStatusProvider("p3", "mock", "z-example.com")
	})

	It("should drop old provider if provider is gone", func() {
		Expect(fakeClient.Create(ctx, entry)).To(Succeed())
		checkEntryStatusProvider("", "", "")

		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady)

		checkEntryStatusProvider("p1", "mock", "z-example.com")

		deleteProvider("p1")
		checkEntryStatusProvider("", "", "z-example.com") // zone should not be removed
	})

	It("should not switch provider if new provider with same domain is added", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady)
		Expect(fakeClient.Create(ctx, entry)).To(Succeed())

		checkEntryStatusProvider("p1", "mock", "z-example.com")

		createProvider("p3", []string{"example.com"}, nil, v1alpha1.StateReady)
		checkEntryStatusProvider("p1", "mock", "z-example.com")
	})

	It("should switch provider if domain name changes", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady)

		Expect(fakeClient.Create(ctx, entry)).To(Succeed())
		checkEntryStatusProvider("p1", "mock", "z-example.com")

		entry.Spec.DNSName = "*.example2.com"
		Expect(fakeClient.Update(ctx, entry)).To(Succeed())
		checkEntryStatusProvider("p2", "mock", "z-example2.com")
	})

	It("should drop provider and zone for unknown domain", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady)

		Expect(fakeClient.Create(ctx, entry)).To(Succeed())
		checkEntryStatusProvider("p1", "mock", "z-example.com")

		entry.Spec.DNSName = "*.unknown-example.com"
		Expect(fakeClient.Update(ctx, entry)).To(Succeed())
		checkEntryStatusProvider("", "", "")
	})

	It("should assign requeue if provider state is missing", func() {
		skipProviderState = true
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady)
		Expect(fakeClient.Create(ctx, entry)).To(Succeed())

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: entryKey})
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, result).To(Equal(reconcile.Result{Requeue: true}))
	})
})
