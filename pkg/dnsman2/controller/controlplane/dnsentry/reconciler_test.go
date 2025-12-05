package dnsentry

import (
	"context"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/lookup"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	dnsprovider "github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/local"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/selection"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// TODO(MartinWeindel) add tests for dynamic changes of CNAME to A lookups
// TODO(MartinWeindel) add tests for alias targets

var _ = Describe("Reconcile", func() {
	var (
		ctx                   = context.Background()
		fakeClient            client.Client
		mlh                   *lookup.MockLookupHost
		cancelLookupProcessor context.CancelFunc
		reconciler            *Reconciler
		registry              *dnsprovider.DNSHandlerRegistry
		accounts              []*dnsprovider.DNSAccount
		entryA                *v1alpha1.DNSEntry
		entryB                *v1alpha1.DNSEntry
		entryC                *v1alpha1.DNSEntry
		clock                 = &testing.FakeClock{}
		startTime             = time.Now().Truncate(time.Second)
		skipProviderState     bool
		defaultTTL            = int64(360)

		mockConfig1 = &local.MockConfig{
			Account: "test",
			Zones: []local.MockZone{
				{DNSName: "example.com"},
				{DNSName: "example2.com"},
			},
		}
		mockConfig1RoutingPolicy = &local.MockConfig{
			Account: "test",
			Zones: []local.MockZone{
				{DNSName: "example.com"},
				{DNSName: "example2.com"},
			},
			SupportRoutingPolicy: true,
		}
		mockConfig2 = &local.MockConfig{
			Account: "test2",
			Zones: []local.MockZone{
				{DNSName: "sub.example.com"},
			},
		}
		zoneID1         = dns.ZoneID{ProviderType: "local", ID: "test:example.com"}
		zoneID2         = dns.ZoneID{ProviderType: "local", ID: "test:example2.com"}
		zoneID3         = dns.ZoneID{ProviderType: "local", ID: "test2:sub.example.com"}
		entryAPrototype = &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "Entry-a",
				Namespace:  "test",
				Generation: 1,
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "test.sub.example.com",
				Targets: []string{"1.2.3.4"},
			},
		}
		entryBPrototype = &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "Entry-b",
				Namespace:  "test",
				Generation: 1,
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "txt.sub.example.com",
				Text:    []string{"This is a text!", "blabla"},
			},
		}
		entryCPrototype = &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "Entry-c",
				Namespace:  "test",
				Generation: 1,
			},
			Spec: v1alpha1.DNSEntrySpec{
				DNSName: "cname.sub.example.com",
				Targets: []string{"www.somewhere.com"},
				TTL:     ptr.To[int64](120),
			},
		}
		recordSetA = dns.RecordSet{
			Type:    dns.TypeA,
			TTL:     defaultTTL,
			Records: []*dns.Record{{Value: "1.2.3.4"}},
		}
		recordSetB = dns.RecordSet{
			Type:    dns.TypeTXT,
			TTL:     defaultTTL,
			Records: []*dns.Record{{Value: "This is a text!"}, {Value: "blabla"}},
		}

		prepareAccount = func(p *v1alpha1.DNSProvider) *dnsprovider.DNSAccount {
			account, err := reconciler.state.GetAccount(log, p, utils.Properties{}, dnsprovider.DNSAccountConfig{
				ZoneCacheTTL: 5 * time.Minute,
				DefaultTTL:   defaultTTL,
				Clock:        clock,
				RateLimits:   nil,
				Factory:      registry,
			})
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			found := false
			for _, acc := range accounts {
				if acc.Hash() == account.Hash() {
					found = true
					break
				}
			}
			if !found {
				accounts = append(accounts, account)
			}

			providerState := reconciler.state.GetOrCreateProviderState(p, config.DNSProviderControllerConfig{Namespace: "test"})
			providerState.SetAccount(account)
			return account
		}
		createProvider = func(name string, included, excluded []string, providerState string, ptrMockConfig *local.MockConfig) *v1alpha1.DNSProvider {
			provider := &v1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "test",
				},
				Spec: v1alpha1.DNSProviderSpec{
					Type:    "local",
					Domains: &v1alpha1.DNSSelection{Include: included, Exclude: excluded},
				},
				Status: v1alpha1.DNSProviderStatus{
					State:              providerState,
					LastUpdateTime:     &metav1.Time{Time: startTime},
					ObservedGeneration: 1,
					Domains:            v1alpha1.DNSSelectionStatus{Included: included, Excluded: excluded},
				},
			}
			if ptrMockConfig != nil {
				var err error
				provider.Spec.ProviderConfig, err = local.MarshallMockConfig(*ptrMockConfig)
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
				prepareAccount(provider)
			}
			if !skipProviderState {
				var zones []selection.LightDNSHostedZone
				for i, account := range accounts {
					accountZones, err := account.GetZones(ctx)
					ExpectWithOffset(1, err).ToNot(HaveOccurred(), "failed to get zones for account %d", i)
					for _, zone := range accountZones {
						zones = append(zones, zone)
					}
				}
				providerState := state.GetState().GetOrCreateProviderState(provider, config.DNSProviderControllerConfig{})
				selectionResult := selection.CalcZoneAndDomainSelection(provider.Spec, zones)
				selectionResult.SetProviderStatusZonesAndDomains(&provider.Status)
				providerState.SetSelection(selectionResult)
				providerState.SetReconciled()
			}
			ExpectWithOffset(1, fakeClient.Create(ctx, provider)).To(Succeed())
			return provider
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
		updateProvider = func(name string, included, excluded []string, state string, ptrMockConfig *local.MockConfig) {
			deleteProvider(name)
			createProvider(name, included, excluded, state, ptrMockConfig)
		}
		reconcileEntry = func(key client.ObjectKey) (reconcile.Result, error) {
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			if result.RequeueAfter == reconciler.reconciliationDelayAfterUpdate {
				time.Sleep(reconciler.reconciliationDelayAfterUpdate * 10)
				result, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			}
			return result, err
		}
		checkEntryStatus = func(entry *v1alpha1.DNSEntry, providerName string, zoneID dns.ZoneID, state string, ttl int64, targets ...string) {
			key := client.ObjectKeyFromObject(entry)
			result, err := reconcileEntry(key)
			ExpectWithOffset(1, fakeClient.Get(ctx, key, entry)).To(Succeed())
			ExpectWithOffset(1, entry.Status.State).To(Equal(state), "state should match")
			if state != "Stale" || entry.Status.Provider == nil {
				ExpectWithOffset(1, err).ToNot(HaveOccurred(), "failed to reconcile Entry")
				ExpectWithOffset(1, result).To(Equal(reconcile.Result{}))
			} else {
				ExpectWithOffset(1, err).ToNot(HaveOccurred(), "expected successful reconciliation with status update")
				ExpectWithOffset(1, entry.Status.Message).NotTo(BeNil(), "expected non-empty status message")
			}
			var ptrName, ptrType, ptrZoneID *string
			if providerName != "" {
				ptrName = ptr.To(providerName)
			}
			if zoneID.ProviderType != "" {
				ptrType = ptr.To(zoneID.ProviderType)
			}
			if zoneID.ID != "" {
				ptrZoneID = ptr.To(zoneID.ID)
			}
			ExpectWithOffset(1, entry.Status.Provider).To(Equal(ptrName), "provider name should match")
			ExpectWithOffset(1, entry.Status.ProviderType).To(Equal(ptrType), "provider type should match")
			ExpectWithOffset(1, entry.Status.Zone).To(Equal(ptrZoneID), "zone ID should match")
			ExpectWithOffset(1, entry.Status.ObservedGeneration).To(Equal(entry.Generation), "observed generation should match")
			var ptrTTL *int64
			if ttl > 0 {
				ptrTTL = ptr.To(ttl)
			}
			ExpectWithOffset(1, entry.Status.TTL).To(Equal(ptrTTL), "TTL should match")

			if len(entry.Status.Targets) == 0 {
				ExpectWithOffset(1, entry.Finalizers).To(BeEmpty())
			} else {
				ExpectWithOffset(1, entry.Finalizers).To(Equal([]string{"dns.gardener.cloud/compound"}))
			}
			ExpectWithOffset(1, entry.Status.Targets).To(Equal(targets), "targets should match")
		}
		checkEntryStatusDeleted = func(entry *v1alpha1.DNSEntry) {
			key := client.ObjectKeyFromObject(entry)
			Expect(fakeClient.Get(ctx, key, entry)).To(Succeed(), "Entry should still exist because of finalizer")
			result, err := reconcileEntry(key)
			ExpectWithOffset(1, err).ToNot(HaveOccurred(), "failed to reconcile Entry on deletion")
			ExpectWithOffset(1, result).To(Equal(reconcile.Result{}))
			ExpectWithOffset(1, apierrors.IsNotFound(fakeClient.Get(ctx, key, entry))).To(BeTrue(), "Entry should be deleted")
		}
		checkEntryStatusRoutingPolicy = func(entry *v1alpha1.DNSEntry) {
			ExpectWithOffset(1, entry.Status.RoutingPolicy).To(Equal(entry.Spec.RoutingPolicy), "routing policy status does not match spec")
		}
		checkEntryStateAndMessage = func(entry *v1alpha1.DNSEntry, expectedState string, messageMatcher types.GomegaMatcher) {
			ExpectWithOffset(1, fakeClient.Get(ctx, client.ObjectKeyFromObject(entry), entry)).To(Succeed())
			ExpectWithOffset(1, entry.Status.State).To(Equal(expectedState), "state should match")
			ExpectWithOffset(1, entry.Status.Message).To(PointTo(messageMatcher), "message should match")
		}
		expectRecordSetsInternal = func(zoneID dns.ZoneID, dnsSetName dns.DNSSetName, expectedNameCount, expectedRecordSetCount int, rsArray ...dns.RecordSet) {
			zoneState := local.GetInMemoryMockByZoneID(zoneID)
			ExpectWithOffset(2, zoneState).ToNot(BeNil(), "zone state should not be nil for zone %s", zoneID)
			nameCount, recordSetCount := zoneState.GetCounts(zoneID)
			ExpectWithOffset(2, nameCount).To(Equal(expectedNameCount), "unexpected name count")
			ExpectWithOffset(2, recordSetCount).To(Equal(expectedRecordSetCount), "unexpected record set count")
			for _, rs := range rsArray {
				actual := zoneState.GetRecordset(zoneID, dnsSetName, rs.Type)
				ExpectWithOffset(2, actual).NotTo(BeNil(), "record set should not be nil for %s %s", dnsSetName, rs.Type)
				ExpectWithOffset(2, *actual).To(Equal(rs))
			}
		}
		expectRecordSets = func(zoneID dns.ZoneID, dnsName string, rsArray ...dns.RecordSet) {
			expectedNameCount := 1
			expectedRecordSetCount := len(rsArray)
			if len(rsArray) == 0 {
				expectedNameCount = 0
			}
			expectRecordSetsInternal(zoneID, dns.DNSSetName{DNSName: dnsName}, expectedNameCount, expectedRecordSetCount, rsArray...)
		}
		expectRecordSetsWithSetIdentifier = func(zoneID dns.ZoneID, dnsName, setIdentifier string, expectedNameCount, expectedRecordSetCount int, rsArray ...dns.RecordSet) {
			expectRecordSetsInternal(zoneID, dns.DNSSetName{DNSName: dnsName, SetIdentifier: setIdentifier}, expectedNameCount, expectedRecordSetCount, rsArray...)
		}
	)

	BeforeEach(func() {
		if registry == nil {
			registry = dnsprovider.NewDNSHandlerRegistry(clock)
			local.RegisterTo(registry)
		}
		clock.SetTime(startTime)
		fakeClient = fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).WithStatusSubresource(&v1alpha1.DNSEntry{}).Build()

		reconciler = &Reconciler{
			Client:          fakeClient,
			Namespace:       "test",
			Clock:           clock,
			state:           state.GetState(),
			lookupProcessor: lookup.NewLookupProcessor(log.WithName("lookup-processor"), lookup.NewNullTrigger(), 1, 250*time.Millisecond),
		}
		reconciler.setReconciliationDelayAfterUpdate(1 * time.Microsecond)
		state.GetState().SetDNSHandlerFactory(registry)

		mlh = lookup.NewMockLookupHost(map[string]lookup.MockLookupHostResult{
			"service-1.example.com": {
				IPs: []net.IP{net.ParseIP("127.0.1.1"), net.ParseIP("2001:db8::1:1")},
			},
			"service-2.example.com": {
				IPs: []net.IP{net.ParseIP("127.0.2.1"), net.ParseIP("127.0.2.2")},
			},
		})

		lookup.SetLookupFunc(mlh.LookupHost)
		lookupCtx, ctxCancel := context.WithCancel(ctx)

		cancelLookupProcessor = func() {
			mlh.Stop()
			ctxCancel()
			Eventually(reconciler.lookupProcessor.IsRunning).WithPolling(1 * time.Millisecond).WithTimeout(100 * time.Millisecond).Should(BeFalse())
		}
		go reconciler.lookupProcessor.Run(lookupCtx)

		Expect(fakeClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}})).To(Succeed())

		entryA = entryAPrototype.DeepCopy()
		entryB = entryBPrototype.DeepCopy()
		entryC = entryCPrototype.DeepCopy()
	})

	AfterEach(func() {
		state.ClearState()
		for _, account := range accounts {
			account.Release()
		}
		accounts = nil
		if cancelLookupProcessor != nil {
			cancelLookupProcessor()
			cancelLookupProcessor = nil
		}
	})

	It("should assign correct provider", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady, mockConfig1)
		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)
	})

	It("should create/update/delete A/AAAA record", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

		entryA.Spec.Targets = []string{"5.6.7.8", "1234::5678"}
		Expect(fakeClient.Update(ctx, entryA)).To(Succeed())
		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "5.6.7.8", "1234::5678")
		expectRecordSets(zoneID1, "test.sub.example.com",
			dns.RecordSet{
				Type:    dns.TypeA,
				TTL:     defaultTTL,
				Records: []*dns.Record{{Value: "5.6.7.8"}},
			},
			dns.RecordSet{
				Type:    dns.TypeAAAA,
				TTL:     defaultTTL,
				Records: []*dns.Record{{Value: "1234::5678"}},
			},
		)

		Expect(fakeClient.Delete(ctx, entryA)).To(Succeed())
		checkEntryStatusDeleted(entryA)
		expectRecordSets(zoneID1, "test.sub.example.com")

		Expect(apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKeyFromObject(entryA), entryA))).To(BeTrue(), "Entry should be not found")
	})

	It("should create/update/delete TXT record", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		Expect(fakeClient.Create(ctx, entryB)).To(Succeed())

		checkEntryStatus(entryB, "test/p1", zoneID1, "Ready", defaultTTL, "\"This is a text!\"", "\"blabla\"")
		expectRecordSets(zoneID1, "txt.sub.example.com", recordSetB)

		entryB.Spec.Text = []string{"This is the *UPDATED* text!!!!"}
		Expect(fakeClient.Update(ctx, entryB)).To(Succeed())
		checkEntryStatus(entryB, "test/p1", zoneID1, "Ready", defaultTTL, "\"This is the *UPDATED* text!!!!\"")
		expectRecordSets(zoneID1, "txt.sub.example.com", dns.RecordSet{
			Type:    dns.TypeTXT,
			TTL:     defaultTTL,
			Records: []*dns.Record{{Value: "This is the *UPDATED* text!!!!"}},
		})

		Expect(fakeClient.Delete(ctx, entryB)).To(Succeed())
		checkEntryStatusDeleted(entryB)
		expectRecordSets(zoneID1, "txt.sub.example.com")

		Expect(apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKeyFromObject(entryB), entryB))).To(BeTrue(), "Entry should be not found")
	})

	It("should create correct CNAME record", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		Expect(fakeClient.Create(ctx, entryC)).To(Succeed())

		checkEntryStatus(entryC, "test/p1", zoneID1, "Ready", 120, "www.somewhere.com")
	})

	It("should create Entry with multiple CNAME targets and resolve the addresses", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		entryC.Spec.Targets = []string{"service-1.example.com", "service-2.example.com"}
		Expect(fakeClient.Create(ctx, entryC)).To(Succeed())

		checkEntryStatus(entryC, "test/p1", zoneID1, "Ready", 120, "127.0.1.1", "127.0.2.1", "127.0.2.2", "2001:db8::1:1")
	})

	It("should create Entry with single CNAME target and resolve the address as configured", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		entryC.Spec.Targets = []string{"service-1.example.com"}
		entryC.Spec.ResolveTargetsToAddresses = ptr.To(true)
		Expect(fakeClient.Create(ctx, entryC)).To(Succeed())

		checkEntryStatus(entryC, "test/p1", zoneID1, "Ready", 120, "127.0.1.1", "2001:db8::1:1")
	})

	It("should update TTL", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

		entryA.Spec.TTL = ptr.To[int64](120)
		Expect(fakeClient.Update(ctx, entryA)).To(Succeed())

		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", 120, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", dns.RecordSet{
			Type:    dns.TypeA,
			TTL:     120,
			Records: []*dns.Record{{Value: "1.2.3.4"}},
		})
	})

	It("should work correctly if record type is changed", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

		entryA.Spec.Targets = []string{"1234::5678"} // change to AAAA record
		Expect(fakeClient.Update(ctx, entryA)).To(Succeed())

		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1234::5678")
		expectRecordSets(zoneID1, "test.sub.example.com", dns.RecordSet{
			Type:    dns.TypeAAAA,
			TTL:     defaultTTL,
			Records: []*dns.Record{{Value: "1234::5678"}},
		})
	})

	It("should assign best matching provider", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady, mockConfig1)
		createProvider("p3", []string{"sub.example.com"}, nil, v1alpha1.StateReady, mockConfig2)
		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

		checkEntryStatus(entryA, "test/p3", zoneID3, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID3, "test.sub.example.com", recordSetA)

		By("should reassign second best provider after removing the best one / cross-zone assignment")
		deleteProvider("p3")
		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID3, "test.sub.example.com")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

		By("should reassign better provider")
		createProvider("p3", []string{"sub.example.com"}, nil, v1alpha1.StateReady, mockConfig2)
		checkEntryStatus(entryA, "test/p3", zoneID3, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com")
		expectRecordSets(zoneID3, "test.sub.example.com", recordSetA)

		By("should reassign after old provider excludes domain")
		updateProvider("p3", []string{"sub.example.com"}, []string{"test.sub.example.com"}, v1alpha1.StateReady, mockConfig2)
		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID3, "test.sub.example.com")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)
	})

	It("should keep old provider if provider changes state and no other provider is matching", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady, mockConfig1)
		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

		By("should keep old provider if it changes state to error")
		updateProvider("p1", []string{"example.com"}, nil, v1alpha1.StateError, mockConfig1)
		checkEntryStatus(entryA, "test/p1", zoneID1, "Stale", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

		By("but should change to new matching, ready provider")
		createProvider("p3", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		checkEntryStatus(entryA, "test/p3", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)
	})

	It("should drop old provider if provider is gone", func() {
		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())
		checkEntryStatus(entryA, "", dns.ZoneID{}, "Error", 0)

		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady, mockConfig1)

		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

		deleteProvider("p1")
		checkEntryStatus(entryA, "", zoneID1, "Stale", defaultTTL, "1.2.3.4") // the zone should not be removed
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)
	})

	It("should not switch provider if new provider with same domain is added", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady, mockConfig1)
		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

		createProvider("p4", []string{"example.com"}, nil, v1alpha1.StateReady, nil)
		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)
	})

	It("should switch provider if domain name changes", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		createProvider("p2", []string{"example2.com"}, nil, v1alpha1.StateReady, mockConfig1)

		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())
		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)
		expectRecordSets(zoneID2, "*.example2.com")

		entryA.Spec.DNSName = "*.example2.com"
		Expect(fakeClient.Update(ctx, entryA)).To(Succeed())
		checkEntryStatus(entryA, "test/p2", zoneID2, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com")
		expectRecordSets(zoneID2, "*.example2.com", recordSetA)
	})

	It("should drop provider and zone for unknown domain", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)

		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())
		checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

		entryA.Spec.DNSName = "*.unknown-example.com"
		Expect(fakeClient.Update(ctx, entryA)).To(Succeed())
		checkEntryStatus(entryA, "", zoneID1, "Stale", defaultTTL, "1.2.3.4")
		expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)
		Expect(entryA.Status.Message).To(Equal(ptr.To("no matching DNS provider found")))
	})

	It("should go to error for unknown domain", func() {
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		entryA.Spec.DNSName = "*.unknown-example.com"
		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())
		checkEntryStatus(entryA, "", dns.ZoneID{}, "Error", 0)
		Expect(entryA.Status.Message).To(Equal(ptr.To("no matching DNS provider found")))
	})

	It("should assign requeue if provider state is missing", func() {
		skipProviderState = true
		defer func() {
			skipProviderState = false
		}()
		createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, nil)
		Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(entryA)})
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, result).To(Equal(reconcile.Result{RequeueAfter: 3 * time.Second}))
	})

	Context("DNSEntry validation", func() {
		BeforeEach(func() {
			createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
		})

		It("rejects an invalid DNSName", func() {
			entryA.Spec.DNSName = "foo_bar.example.com"
			Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(entryA)})
			Expect(err).To(Succeed())
			checkEntryStateAndMessage(entryA, "Invalid", ContainSubstring("validation failed: invalid DNSName:"))
		})

		It("rejects setting both Targets and Text", func() {
			entryA.Spec.Targets = []string{"1.1.1.1"}
			entryA.Spec.Text = []string{"foo"}
			Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(entryA)})
			Expect(err).To(Succeed())
			checkEntryStateAndMessage(entryA, "Invalid", ContainSubstring("validation failed: cannot specify both targets and text fields"))
		})

		It("rejects an empty target", func() {
			entryA.Spec.Targets = []string{""}
			Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(entryA)})
			Expect(err).To(Succeed())
			checkEntryStateAndMessage(entryA, "Invalid", ContainSubstring("validation failed: target 1 is empty"))
		})

		It("rejects duplicate targets", func() {
			entryA.Spec.Targets = []string{"1.1.1.1", "1.1.1.1"}
			Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(entryA)})
			Expect(err).To(Succeed())
			checkEntryStateAndMessage(entryA, "Invalid", ContainSubstring("validation failed: target 2 is a duplicate: 1.1.1.1"))
		})

		It("rejects an empty text", func() {
			entryB.Spec.Text = []string{""}
			Expect(fakeClient.Create(ctx, entryB)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(entryB)})
			Expect(err).To(Succeed())
			checkEntryStateAndMessage(entryB, "Invalid", ContainSubstring("validation failed: text 1 is empty"))
		})

		It("rejects duplicate text", func() {
			entryB.Spec.Text = []string{"foo", "foo"}
			Expect(fakeClient.Create(ctx, entryB)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(entryB)})
			Expect(err).To(Succeed())
			checkEntryStateAndMessage(entryB, "Invalid", ContainSubstring("validation failed: text 2 is a duplicate: foo"))
		})
	})

	DescribeTable("should respect ignore annotations",
		func(ignoreAnnotations map[string]string, shouldKeepRecords bool) {
			createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
			entryA = entryAPrototype.DeepCopy()
			Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

			checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
			expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

			entryA.Annotations = ignoreAnnotations
			entryA.Spec.Targets = []string{"5.6.7.8"}
			Expect(fakeClient.Update(ctx, entryA)).To(Succeed())
			checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
			expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

			Expect(fakeClient.Delete(ctx, entryA)).To(Succeed())
			checkEntryStatusDeleted(entryA)
			if shouldKeepRecords {
				expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)
			} else {
				expectRecordSets(zoneID1, "test.sub.example.com")
			}
			Expect(apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKeyFromObject(entryA), entryA))).To(BeTrue(), "Entry should be not found")
		},
		Entry("ignore=true", map[string]string{dns.AnnotationIgnore: "true"}, false),
		Entry("ignore=reconcile", map[string]string{dns.AnnotationIgnore: "reconcile"}, false),
		Entry("ignore=full", map[string]string{dns.AnnotationIgnore: "full"}, true),
		Entry("target-hard-ignore=true", map[string]string{dns.AnnotationHardIgnore: "true"}, true),
	)

	Context("Routing policy", func() {
		It("should create/update/delete A/AAAA record with routing policy", func() {
			createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1RoutingPolicy)
			rp := dns.RoutingPolicy{
				Type:       dns.RoutingPolicyWeighted,
				Parameters: map[string]string{"weight": "50"},
			}
			entryA.Spec.RoutingPolicy = &v1alpha1.RoutingPolicy{
				Type:          string(rp.Type),
				SetIdentifier: "set1",
				Parameters:    rp.Parameters,
			}
			Expect(fakeClient.Create(ctx, entryA)).To(Succeed())

			entryA2 := entryAPrototype.DeepCopy()
			entryA2.Name = entryA2.Name + "-2"
			entryA2.Spec.Targets = []string{"10.20.30.40"}
			entryA2.Spec.RoutingPolicy = &v1alpha1.RoutingPolicy{
				Type:          string(rp.Type),
				SetIdentifier: "set2",
				Parameters:    rp.Parameters,
			}
			Expect(fakeClient.Create(ctx, entryA2)).To(Succeed())

			checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
			checkEntryStatusRoutingPolicy(entryA)
			expectRecordSetsWithSetIdentifier(zoneID1, "test.sub.example.com", "set1", 1, 1, dns.RecordSet{
				Type:          dns.TypeA,
				TTL:           defaultTTL,
				Records:       []*dns.Record{{Value: "1.2.3.4"}},
				RoutingPolicy: &rp,
			})
			checkEntryStatus(entryA2, "test/p1", zoneID1, "Ready", defaultTTL, "10.20.30.40")
			checkEntryStatusRoutingPolicy(entryA2)
			expectRecordSetsWithSetIdentifier(zoneID1, "test.sub.example.com", "set2", 2, 2, dns.RecordSet{
				Type:          dns.TypeA,
				TTL:           defaultTTL,
				Records:       []*dns.Record{{Value: "10.20.30.40"}},
				RoutingPolicy: &rp,
			})

			entryA.Spec.Targets = []string{"5.6.7.8", "1234::5678"}
			rp2 := dns.RoutingPolicy{
				Type:       dns.RoutingPolicyWeighted,
				Parameters: map[string]string{"weight": "10"},
			}
			entryA.Spec.RoutingPolicy.Parameters = rp2.Parameters
			Expect(fakeClient.Update(ctx, entryA)).To(Succeed())
			checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "5.6.7.8", "1234::5678")
			checkEntryStatusRoutingPolicy(entryA)
			expectRecordSetsWithSetIdentifier(zoneID1, "test.sub.example.com", "set1", 2, 3,
				dns.RecordSet{
					Type:          dns.TypeA,
					TTL:           defaultTTL,
					Records:       []*dns.Record{{Value: "5.6.7.8"}},
					RoutingPolicy: &rp2,
				},
				dns.RecordSet{
					Type:          dns.TypeAAAA,
					TTL:           defaultTTL,
					Records:       []*dns.Record{{Value: "1234::5678"}},
					RoutingPolicy: &rp2,
				},
			)

			Expect(fakeClient.Delete(ctx, entryA)).To(Succeed())
			checkEntryStatusDeleted(entryA)
			expectRecordSetsWithSetIdentifier(zoneID1, "test.sub.example.com", "set1", 1, 1)

			checkEntryStatus(entryA2, "test/p1", zoneID1, "Ready", defaultTTL, "10.20.30.40")
			expectRecordSetsWithSetIdentifier(zoneID1, "test.sub.example.com", "set2", 1, 1, dns.RecordSet{
				Type:          dns.TypeA,
				TTL:           defaultTTL,
				Records:       []*dns.Record{{Value: "10.20.30.40"}},
				RoutingPolicy: &rp,
			})

			Expect(fakeClient.Delete(ctx, entryA2)).To(Succeed())
			checkEntryStatusDeleted(entryA2)
			expectRecordSetsWithSetIdentifier(zoneID1, "test.sub.example.com", "set2", 0, 0)

			Expect(apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKeyFromObject(entryA), entryA))).To(BeTrue(), "Entry should be not found")
			Expect(apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKeyFromObject(entryA2), entryA2))).To(BeTrue(), "Entry should be not found")
		})

		It("should update Entry by adding set identifier and routing policy", func() {
			createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1RoutingPolicy)
			Expect(fakeClient.Create(ctx, entryA)).To(Succeed())
			checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
			expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)

			rp := dns.RoutingPolicy{
				Type:       dns.RoutingPolicyWeighted,
				Parameters: map[string]string{"weight": "50"},
			}
			entryA.Spec.RoutingPolicy = &v1alpha1.RoutingPolicy{
				Type:          string(rp.Type),
				SetIdentifier: "set1",
				Parameters:    rp.Parameters,
			}
			Expect(fakeClient.Update(ctx, entryA)).To(Succeed())
			checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
			expectRecordSetsWithSetIdentifier(zoneID1, "test.sub.example.com", "", 1, 1)
			expectRecordSetsWithSetIdentifier(zoneID1, "test.sub.example.com", "set1", 1, 1, dns.RecordSet{
				Type:          dns.TypeA,
				TTL:           defaultTTL,
				Records:       []*dns.Record{{Value: "1.2.3.4"}},
				RoutingPolicy: &rp,
			})
		})

		It("should update Entry by removing set identifier and routing policy", func() {
			createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1RoutingPolicy)

			rp := dns.RoutingPolicy{
				Type:       dns.RoutingPolicyWeighted,
				Parameters: map[string]string{"weight": "50"},
			}
			entryA.Spec.RoutingPolicy = &v1alpha1.RoutingPolicy{
				Type:          string(rp.Type),
				SetIdentifier: "set1",
				Parameters:    rp.Parameters,
			}
			Expect(fakeClient.Create(ctx, entryA)).To(Succeed())
			checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
			checkEntryStatusRoutingPolicy(entryA)
			expectRecordSetsWithSetIdentifier(zoneID1, "test.sub.example.com", "set1", 1, 1, dns.RecordSet{
				Type:          dns.TypeA,
				TTL:           defaultTTL,
				Records:       []*dns.Record{{Value: "1.2.3.4"}},
				RoutingPolicy: &rp,
			})

			entryA.Spec.RoutingPolicy = nil
			Expect(fakeClient.Update(ctx, entryA)).To(Succeed())
			checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
			checkEntryStatusRoutingPolicy(entryA)
			expectRecordSets(zoneID1, "test.sub.example.com", recordSetA)
		})
	})

	Context("Migration Mode", func() {
		It("should ignore entry status if provider type is 'remote'", func() {
			createProvider("p1", []string{"example.com"}, nil, v1alpha1.StateReady, mockConfig1)
			Expect(fakeClient.Create(ctx, entryA)).To(Succeed())
			checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")

			entryA.Status.ProviderType = ptr.To("remote")
			Expect(fakeClient.Status().Update(ctx, entryA)).To(Succeed())
			key := client.ObjectKeyFromObject(entryA)
			result, err := reconcileEntry(key)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
			Expect(fakeClient.Get(ctx, key, entryA)).To(Succeed())
			checkEntryStateAndMessage(entryA, "Error", Equal("failed to map old targets: provider type \"remote\" not found in registry"))

			reconciler.MigrationMode = true
			checkEntryStatus(entryA, "test/p1", zoneID1, "Ready", defaultTTL, "1.2.3.4")
		})
	})
})
