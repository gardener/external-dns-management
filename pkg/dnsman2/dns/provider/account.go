// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/metrics"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// DNSAccountConfig holds configuration for a DNSAccount.
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
	// GlobalConfig is the global DNS manager configuration.
	GlobalConfig *config.DNSManagerConfiguration
}

// DNSAccount represents a DNS account.
type DNSAccount struct {
	*utils.RateLimiter
	handler DNSHandler
	hash    string
	config  DNSAccountConfig

	lock         sync.Mutex
	dnsCaches    map[dns.ZoneID]*utils.DNSCache
	cachedZones  []DNSHostedZone
	lastGetZones time.Time
	clients      sets.Set[client.ObjectKey]
}

var (
	_ DNSHandler = &DNSAccount{}
	_ Metrics    = &DNSAccount{}
)

type handlerZoneQueryDNS struct {
	queryFunc CustomQueryDNSFunc
	zone      dns.ZoneInfo
}

var _ utils.QueryDNS = &handlerZoneQueryDNS{}

func (h *handlerZoneQueryDNS) Query(ctx context.Context, setName dns.DNSSetName, recordType dns.RecordType) utils.QueryDNSResult {
	rs, err := h.queryFunc(ctx, h.zone, setName, recordType)
	if err != nil {
		return utils.QueryDNSResult{Err: err}
	}
	return utils.QueryDNSResult{RecordSet: rs, Err: err}
}

// NewDNSAccount creates a new DNSAccount with the given handler, hash, and config.
func NewDNSAccount(handler DNSHandler, hash string, config DNSAccountConfig) *DNSAccount {
	return &DNSAccount{
		RateLimiter: utils.NewRateLimiter(3*time.Second, 10*time.Minute),
		handler:     handler,
		hash:        hash,
		config:      config,
		clients:     sets.New[client.ObjectKey](),
		dnsCaches:   map[dns.ZoneID]*utils.DNSCache{},
	}
}

// AddGenericRequests adds generic request metrics for the account.
func (a *DNSAccount) AddGenericRequests(requestType MetricsRequestType, n int) {
	metrics.AddRequests(a.handler.ProviderType(), a.hash, string(requestType), n, nil)
}

// AddZoneRequests adds zone-specific request metrics for the account.
func (a *DNSAccount) AddZoneRequests(zoneID string, requestType MetricsRequestType, n int) {
	metrics.AddRequests(a.handler.ProviderType(), a.hash, string(requestType), n, &zoneID)
}

// ProviderType returns the provider type of the DNS account.
func (a *DNSAccount) ProviderType() string {
	return a.handler.ProviderType()
}

// Hash returns the hash of the DNS account.
func (a *DNSAccount) Hash() string {
	return a.hash
}

// GetZones returns the hosted zones for the DNS account, using a cache.
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

// GetCustomQueryDNSFunc returns a custom query DNS function for the given zone.
func (a *DNSAccount) GetCustomQueryDNSFunc(zone dns.ZoneInfo, factory utils.QueryDNSFactoryFunc) (CustomQueryDNSFunc, error) {
	return a.handler.GetCustomQueryDNSFunc(zone, factory)
}

func (a *DNSAccount) getZoneQueryCache(ctx context.Context, zone dns.ZoneInfo) (*utils.DNSCache, error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	cache, ok := a.dnsCaches[zone.ZoneID()]
	if !ok {
		factory := func() (utils.QueryDNS, error) {
			nsProvider, err := utils.NewHostedZoneNameserversProvider(ctx, zone.Domain(), 12*time.Hour, utils.SystemNameservers) // TODO make minRefreshPeriod configurable
			if err != nil {
				return nil, fmt.Errorf("failed to create nameservers provider for zone %s: %w", zone.ZoneID(), err)
			}
			return utils.NewStandardQueryDNS(nsProvider), nil
		}

		queryFunc, err := a.handler.GetCustomQueryDNSFunc(zone, factory)
		if err != nil {
			return nil, fmt.Errorf("failed to get custom query DNS function for zone %s: %w", zone.ZoneID(), err)
		}
		if queryFunc != nil {
			cache = utils.NewDNSCache(&handlerZoneQueryDNS{queryFunc: queryFunc, zone: zone}, 30*time.Second) // TODO set default TTL
		} else {
			dnsQuery, err := factory()
			if err != nil {
				return nil, err
			}
			cache = utils.NewDNSCache(dnsQuery, 30*time.Second) // TODO set default TTL
		}
		a.dnsCaches[zone.ZoneID()] = cache
	}
	return cache, nil
}

func (a *DNSAccount) cleanZoneQueryCache(zones []DNSHostedZone) {
	zoneSet := sets.New[dns.ZoneID]()
	for _, zone := range zones {
		zoneSet.Insert(zone.ZoneID())
	}
	for zoneID := range a.dnsCaches {
		if !zoneSet.Has(zoneID) {
			delete(a.dnsCaches, zoneID)
		}
	}
}

// ExecuteRequests executes DNS change requests for the given zone.
func (a *DNSAccount) ExecuteRequests(ctx context.Context, zone DNSHostedZone, requests ChangeRequests) error {
	return a.handler.ExecuteRequests(ctx, zone, requests)
}

// Release releases the DNS account and its handler.
func (a *DNSAccount) Release() {
	a.handler.Release()
}

// AccountMap manages a set of DNS accounts.
type AccountMap struct {
	lock     sync.Mutex
	accounts map[string]*DNSAccount
}

// NewAccountMap creates a new AccountMap.
func NewAccountMap() *AccountMap {
	return &AccountMap{
		accounts: map[string]*DNSAccount{},
	}
}

// Get returns a DNSAccount for the given provider, creating it if necessary.
func (m *AccountMap) Get(log logr.Logger, provider *v1alpha1.DNSProvider, props utils.Properties, accountConfig DNSAccountConfig) (*DNSAccount, error) {
	key := client.ObjectKeyFromObject(provider)
	hash := m.Hash(props, provider.Spec.Type, provider.Spec.ProviderConfig)
	m.lock.Lock()
	defer m.lock.Unlock()

	a := m.accounts[hash]
	if a == nil {
		a = NewDNSAccount(nil, hash, accountConfig)

		rateLimiter, err := NewRateLimiter(accountConfig.RateLimits)
		if err != nil {
			return nil, err
		}

		cfg := DNSHandlerConfig{
			Log:          log,
			Properties:   props,
			Config:       provider.Spec.ProviderConfig,
			GlobalConfig: ptr.Deref(accountConfig.GlobalConfig, config.DNSManagerConfiguration{}),
			Metrics:      a,
			RateLimiter:  rateLimiter,
		}
		a.handler, err = accountConfig.Factory.Create(provider.Spec.Type, &cfg)
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

// FindAccountForZone finds the DNSAccount and DNSHostedZone for a given zone ID.
func (m *AccountMap) FindAccountForZone(ctx context.Context, zoneID dns.ZoneID) (*DNSAccount, *DNSHostedZone, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var errs []error

	for _, account := range m.accounts {
		if account.ProviderType() != zoneID.ProviderType {
			continue
		}

		zones, err := account.GetZones(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get zones for account %s: %w", account.Hash(), err))
			continue
		}
		for _, zone := range zones {
			if zone.ZoneID() == zoneID {
				return account, &zone, nil
			}
		}
	}
	errs = append([]error{fmt.Errorf("no account found for zone %s", zoneID)}, errs...)
	return nil, nil, errors.Join(errs...)
}

var null = []byte{0}

// Release releases a DNSAccount for a given provider key.
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

// Hash computes a hash for the given properties, provider type, and provider config.
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

// GetDNSCachesByZone returns all DNS caches for a given zone ID.
func (m *AccountMap) GetDNSCachesByZone(ctx context.Context, zoneID dns.ZoneID) ([]*utils.DNSCache, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var result []*utils.DNSCache
	for _, account := range m.accounts {
		for _, zone := range account.cachedZones {
			if zone.ZoneID() == zoneID {
				zoneInfo := dns.NewZoneInfo(zone.ZoneID(), zone.Domain(), zone.IsPrivate(), zone.Key())
				cache, err := account.getZoneQueryCache(ctx, zoneInfo)
				if err != nil {
					return nil, fmt.Errorf("failed to get DNS cache for zone %s: %w", zoneID, err)
				}
				result = append(result, cache)
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no DNS caches found for zone %s", zoneID)
	}
	return result, nil
}
