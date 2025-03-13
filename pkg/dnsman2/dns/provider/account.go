// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/metrics"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type DNSAccountConfig struct {
	// DefaultTTL is the default TTL for DNS records.
	DefaultTTL int64
	// ZoneCacheTTL is the TTL for the cache for `GetZones` method.
	ZoneCacheTTL time.Duration
	// Factory is the DNS handler factory.
	Factory DNSHandlerFactory
	// Clock is the clock.
	Clock clock.Clock
	// RateLimiterOptions are the rate limiter options.
	RateLimits *config.RateLimiterOptions
}

// DNSAccount represents a DNS account
type DNSAccount struct {
	*utils.RateLimiter
	handler DNSHandler
	hash    string
	config  DNSAccountConfig

	lock         sync.Mutex
	dnsCaches    map[DNSHostedZone]*utils.DNSCache
	cachedZones  []DNSHostedZone
	lastGetZones time.Time
	clients      sets.Set[client.ObjectKey]
}

var (
	_ DNSHandler = &DNSAccount{}
	_ Metrics    = &DNSAccount{}
)

type handlerZoneQueryDNS struct {
	handler DNSHandler
	zone    DNSHostedZone
}

var _ utils.QueryDNS = &handlerZoneQueryDNS{}

func (h *handlerZoneQueryDNS) Query(ctx context.Context, dnsName string, recordType dns.RecordType) utils.QueryDNSResult {
	records, ttl, err := h.handler.QueryDNS(ctx, h.zone, dnsName, recordType)
	if err != nil {
		return utils.QueryDNSResult{Err: err}
	}
	return utils.QueryDNSResult{Records: records, TTL: utils.TTLToUint32(ttl)}
}

func NewDNSAccount(handler DNSHandler, hash string, config DNSAccountConfig) *DNSAccount {
	return &DNSAccount{
		RateLimiter: utils.NewRateLimiter(3*time.Second, 10*time.Minute),
		handler:     handler,
		hash:        hash,
		config:      config,
		clients:     sets.New[client.ObjectKey](),
		dnsCaches:   map[DNSHostedZone]*utils.DNSCache{},
	}
}

func (a *DNSAccount) AddGenericRequests(requestType string, n int) {
	metrics.AddRequests(a.handler.ProviderType(), a.hash, requestType, n, nil)
}

func (a *DNSAccount) AddZoneRequests(zoneID, requestType string, n int) {
	metrics.AddRequests(a.handler.ProviderType(), a.hash, requestType, n, &zoneID)
}

func (a *DNSAccount) ProviderType() string {
	return a.handler.ProviderType()
}

func (a *DNSAccount) Hash() string {
	return a.hash
}

func (a *DNSAccount) GetZones(ctx context.Context) ([]DNSHostedZone, error) {
	a.lock.Lock()
	if a.config.Clock.Since(a.lastGetZones) < a.config.ZoneCacheTTL {
		a.lock.Unlock()
		return a.cachedZones, nil
	}
	defer a.lock.Unlock()

	zones, err := a.handler.GetZones(ctx)
	if err == nil {
		a.cachedZones = zones
		a.lastGetZones = a.config.Clock.Now()
		a.Succeeded()
		a.cleanZoneQueryCache(zones)
	} else {
		a.Failed()
	}
	return zones, err
}

func (a *DNSAccount) QueryDNS(ctx context.Context, zone DNSHostedZone, dnsName string, recordType dns.RecordType) ([]dns.Record, int64, error) {
	cache := a.getZoneQueryCache(zone)
	result := cache.Get(ctx, dnsName, recordType)
	return result.Records, int64(result.TTL), result.Err
}

func (a *DNSAccount) getZoneQueryCache(zone DNSHostedZone) *utils.DNSCache {
	a.lock.Lock()
	defer a.lock.Unlock()

	cache, ok := a.dnsCaches[zone]
	if !ok {
		cache = utils.NewDNSCache(&handlerZoneQueryDNS{handler: a.handler, zone: zone}, 30*time.Second) // TODO set default TTL
		a.dnsCaches[zone] = cache
	}
	return cache
}

func (a *DNSAccount) cleanZoneQueryCache(zones []DNSHostedZone) {
	zoneSet := sets.New[DNSHostedZone](zones...)
	for zone := range a.dnsCaches {
		if !zoneSet.Has(zone) {
			delete(a.dnsCaches, zone)
		}
	}
}

func (a *DNSAccount) ExecuteRequests(ctx context.Context, zone DNSHostedZone, reqs []*ChangeRequest) error {
	return a.handler.ExecuteRequests(ctx, zone, reqs)
}

func (a *DNSAccount) MapTargets(dnsName string, targets []dns.Target) []dns.Target {
	return a.handler.MapTargets(dnsName, targets)
}

func (a *DNSAccount) Release() {
	a.handler.Release()
}

type AccountMap struct {
	lock     sync.Mutex
	accounts map[string]*DNSAccount
}

func NewAccountMap() *AccountMap {
	return &AccountMap{
		accounts: map[string]*DNSAccount{},
	}
}

func (m *AccountMap) Get(log logr.Logger, provider *v1alpha1.DNSProvider, props utils.Properties, config DNSAccountConfig) (*DNSAccount, error) {
	key := client.ObjectKeyFromObject(provider)
	hash := m.Hash(props, provider.Spec.Type, provider.Spec.ProviderConfig)
	m.lock.Lock()
	defer m.lock.Unlock()

	a := m.accounts[hash]
	if a == nil {
		a = NewDNSAccount(nil, hash, config)

		rateLimiter, err := NewRateLimiter(config.RateLimits)
		if err != nil {
			return nil, err
		}

		cfg := DNSHandlerConfig{
			Log:         log,
			Properties:  props,
			Config:      provider.Spec.ProviderConfig,
			Metrics:     a,
			RateLimiter: rateLimiter,
		}
		a.handler, err = config.Factory.Create(provider.Spec.Type, &cfg)
		if err != nil {
			return nil, err
		}
		log.Info("creating account", "provider", key, "hash", a.Hash())
		m.accounts[hash] = a
	}
	old := len(a.clients)
	a.clients.Insert(key)
	if old != len(a.clients) && old != 0 {
		log.Info("reusing account", "provider", key, "hash", a.Hash(), "clients", len(a.clients))
	}
	metrics.ReportAccountProviders(provider.Spec.Type, a.Hash(), len(a.clients))
	return a, nil
}

var null = []byte{0}

func (m *AccountMap) Release(log logr.Logger, a *DNSAccount, key client.ObjectKey) {
	if a == nil {
		return
	}
	m.lock.Lock()
	defer m.lock.Unlock()

	a.clients.Delete(key)
	if len(a.clients) == 0 {
		log.Info("releasing account", "provider", key, "hash", a.Hash())
		delete(m.accounts, a.hash)
		metrics.DeleteAccount(a.ProviderType(), a.Hash())
		a.handler.Release()
	} else {
		log.Info("keeping account after releasing provider", "provider", key, "hash", a.Hash(), "clients", len(a.clients))
	}
}

func (m *AccountMap) Hash(props utils.Properties, ptype string, extension *runtime.RawExtension) string {
	keys := make([]string, len(props))
	i := 0
	h := sha256.New224()
	for k := range props {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := props[k]
		h.Write([]byte(k))
		h.Write(null)
		h.Write(([]byte(v)))
		h.Write(null)
	}

	if extension != nil {
		h.Write(extension.Raw)
	}
	h.Write(null)
	h.Write([]byte(ptype))
	return hex.EncodeToString(h.Sum(nil))
}
