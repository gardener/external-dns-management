package dnsprovider

import (
	"context"
	"time"

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
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	dnsprovider "github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/local"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

var _ = Describe("Reconcile", func() {
	var (
		ctx         = context.Background()
		fakeClient  client.Client
		reconciler  *Reconciler
		providerKey = client.ObjectKey{Name: "provider1", Namespace: "test"}
		provider    *v1alpha1.DNSProvider
		secret1     *corev1.Secret
		factory     *dnsprovider.DNSHandlerRegistry
		clock       = &testing.FakeClock{}
		startTime   = time.Now().Truncate(time.Second)

		checkFailed = func(expectedState string, expectedMessage string) {
			Expect(fakeClient.Get(ctx, providerKey, provider)).To(Succeed())
			Expect(provider.Status.ObservedGeneration).To(Equal(provider.Generation))
			Expect(provider.Status.State).To(Equal(expectedState))
			Expect(provider.Status.LastUpdateTime.Time).To(Equal(startTime))
			Expect(provider.Status.Message).To(Equal(ptr.To(expectedMessage)))
			Expect(provider.Status.Domains).To(Equal(v1alpha1.DNSSelectionStatus{}))
			Expect(provider.Status.Zones).To(Equal(v1alpha1.DNSSelectionStatus{}))
			Expect(provider.Status.DefaultTTL).To(BeNil())
			Expect(provider.Status.RateLimit).To(BeNil())
		}
	)

	BeforeEach(func() {
		clock.SetTime(startTime)
		factory = dnsprovider.NewDNSHandlerRegistry(clock)
		local.RegisterTo(factory)
		state.GetState().SetDNSHandlerFactory(factory)
		fakeClient = fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).WithStatusSubresource(&v1alpha1.DNSProvider{}).Build()
		reconciler = &Reconciler{
			Client: fakeClient,
			Config: config.DNSProviderControllerConfig{
				Namespace:  "test",
				DefaultTTL: ptr.To[int64](300),
			},
			Clock:             clock,
			DNSHandlerFactory: factory,
			state:             state.GetState(),
		}

		secret1 = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secret1", Namespace: providerKey.Namespace},
			Data:       map[string][]byte{"foo": []byte("bar")},
		}
		Expect(fakeClient.Create(ctx, secret1)).To(Succeed())
		rawMockConfig, err := local.MarshallMockConfig(local.MockConfig{
			Account: "test",
			Zones: []local.MockZone{
				{DNSName: "example.com"},
				{DNSName: "example2.com"},
			},
		})
		Expect(err).ToNot(HaveOccurred())
		provider = &v1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:       providerKey.Name,
				Namespace:  providerKey.Namespace,
				Generation: 1,
			},
			Spec: v1alpha1.DNSProviderSpec{
				SecretRef: &corev1.SecretReference{
					Name: "secret1",
				},
				Type:           "local",
				ProviderConfig: rawMockConfig,
				Zones: &v1alpha1.DNSSelection{
					Include: []string{"test:example.com"},
				},
			},
		}
	})

	It("should update status for unsupported provider type", func() {
		provider.Spec.Type = "unsupported"
		Expect(fakeClient.Create(ctx, provider)).To(Succeed())

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: providerKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		checkFailed(v1alpha1.StateInvalid, `provider type "unsupported" is not supported`)
	})

	It("should update status for supported provider type", func() {
		Expect(fakeClient.Create(ctx, provider)).To(Succeed())
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: providerKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		Expect(fakeClient.Get(ctx, providerKey, provider)).To(Succeed())
		Expect(provider.Finalizers).To(ConsistOf("dns.gardener.cloud/compound"))
		Expect(provider.Status.LastUpdateTime.Time).To(Equal(startTime))
		Expect(provider.Status.State).To(Equal(v1alpha1.StateReady))
		Expect(provider.Status.ObservedGeneration).To(Equal(provider.Generation))
		Expect(provider.Status.Domains).To(Equal(v1alpha1.DNSSelectionStatus{Included: []string{"example.com"}, Excluded: []string{"example2.com"}}))
		Expect(provider.Status.Zones).To(Equal(v1alpha1.DNSSelectionStatus{Included: []string{"test:example.com"}, Excluded: []string{"test:example2.com"}}))
		Expect(provider.Status.DefaultTTL).To(Equal(ptr.To[int64](300)))
		Expect(provider.Status.RateLimit).To(BeNil())

		clock.Step(1 * time.Minute)

		By("reconciling again, status should remain unchanged")
		oldStatus := provider.Status.DeepCopy()
		result, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: providerKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(fakeClient.Get(ctx, providerKey, provider)).To(Succeed())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(&provider.Status).To(Equal(oldStatus))

		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(secret1), secret1)).To(Succeed())
		Expect(secret1.Finalizers).To(ConsistOf("dns.gardener.cloud/compound"))
	})

	It("should update status for supported provider type if secret ref namespace is missing", func() {
		provider.Spec.SecretRef.Namespace = ""
		Expect(fakeClient.Create(ctx, provider)).To(Succeed())
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: providerKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		Expect(fakeClient.Get(ctx, providerKey, provider)).To(Succeed())
		Expect(provider.Finalizers).To(ConsistOf("dns.gardener.cloud/compound"))
		Expect(provider.Status.LastUpdateTime.Time).To(Equal(startTime))
		Expect(provider.Status.State).To(Equal(v1alpha1.StateReady))
		Expect(provider.Status.ObservedGeneration).To(Equal(provider.Generation))

		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(secret1), secret1)).To(Succeed())
		Expect(secret1.Finalizers).To(ConsistOf("dns.gardener.cloud/compound"))
	})

	It("should update status for supported provider type if secret ref namespace is missing (variant migration mode)", func() {
		provider.Spec.SecretRef.Namespace = ""
		Expect(fakeClient.Create(ctx, provider)).To(Succeed())
		reconciler.Config.MigrationMode = ptr.To(true)
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: providerKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		Expect(fakeClient.Get(ctx, providerKey, provider)).To(Succeed())
		Expect(provider.Finalizers).To(ConsistOf("dns.gardener.cloud/compound"))
		Expect(provider.Status.LastUpdateTime.Time).To(Equal(startTime))
		Expect(provider.Status.State).To(Equal(v1alpha1.StateReady))
		Expect(provider.Status.ObservedGeneration).To(Equal(provider.Generation))

		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(secret1), secret1)).To(Succeed())
		Expect(secret1.Finalizers).To(BeNil())
	})

	It("should update status for if secretref is not set", func() {
		provider.Spec.SecretRef = nil
		Expect(fakeClient.Create(ctx, provider)).To(Succeed())
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: providerKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		checkFailed(v1alpha1.StateInvalid, "no secret reference specified")
	})

	It("should update status for if secretRef is not existing", func() {
		provider.Spec.SecretRef.Name = "not-existing"
		Expect(fakeClient.Create(ctx, provider)).To(Succeed())
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: providerKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		checkFailed(v1alpha1.StateError, "secret test/not-existing not found")
	})

	It("should update status if account has no zones", func() {
		rawMockConfig, err := local.MarshallMockConfig(local.MockConfig{
			Account: "account1",
			Zones:   []local.MockZone{},
		})
		Expect(err).ToNot(HaveOccurred())
		provider.Spec.ProviderConfig = rawMockConfig
		Expect(fakeClient.Create(ctx, provider)).To(Succeed())
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: providerKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{RequeueAfter: 5 * time.Minute}))

		checkFailed(v1alpha1.StateError, "no hosted zones available in account")
	})

	It("should update status for if validation of secret fails", func() {
		Expect(fakeClient.Update(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secret1", Namespace: providerKey.Namespace},
			Data:       map[string][]byte{"bad_key": []byte("some-value")},
		})).To(Succeed())
		Expect(fakeClient.Create(ctx, provider)).To(Succeed())
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: providerKey})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		checkFailed(v1alpha1.StateError, "secret test/secret1 validation failed: 'bad_key' is not allowed in local provider properties: some-value")
	})
})
