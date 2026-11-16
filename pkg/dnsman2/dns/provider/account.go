// SPDX-FileCopyrightText: Contributors to the Gardener project
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

const defaultDNSCacheTTL = 5 * time.Second

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

func (h *handlerZoneQueryDNS) Query(ctx context.Context, request utils.QueryDNSRequest) utils.QueryDNSResult {
	return h.queryFunc(ctx, h.zone, request)
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

// GetCachedZone returns the cached hosted zone for the given zone ID, or nil if not found.
func (a *DNSAccount) GetCachedZone(zoneID dns.ZoneID) DNSHostedZone {
	a.lock.Lock()
	defer a.lock.Unlock()

	for _, zone := range a.cachedZones {
		if zone.ZoneID() == zoneID {
			return zone
		}
	}
	return nil
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
			nsProvider, err := utils.NewHostedZoneNameserversProvider(ctx, zone.Domain(), 12*time.Hour, utils.SystemNameservers) // TODO make maxCachePeriod configurable
			if err != nil {
				return nil, fmt.Errorf("failed to create nameservers provider for zone %s: %w", zone.ZoneID(), err)
			}
			return utils.NewStandardQueryDNS(nsProvider), nil
		}

		queryFunc, err := a.handler.GetCustomQueryDNSFunc(zone, factory)
		if err != nil {
			return nil, fmt.Errorf("failed to get custom query DNS function for zone %s: %w", zone.ZoneID(), err)
		}
		ttl := defaultDNSCacheTTL
		if a.config.GlobalConfig != nil && a.config.GlobalConfig.Controllers.DNSEntry.PropagationWaitTime != nil {
			ttl = max(a.config.GlobalConfig.Controllers.DNSEntry.PropagationWaitTime.Duration, defaultDNSCacheTTL)
		}
		if queryFunc != nil {
			cache = utils.NewDNSCache(&handlerZoneQueryDNS{queryFunc: queryFunc, zone: zone}, ttl)
		} else {
			dnsQuery, err := factory()
			if err != nil {
				return nil, err
			}
			cache = utils.NewDNSCache(dnsQuery, ttl)
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
	lock       sync.Mutex
	accounts   map[string]*DNSAccount
	inCreation map[string]chan struct{}
}

// NewAccountMap creates a new AccountMap.
func NewAccountMap() *AccountMap {
	return &AccountMap{
		accounts:   map[string]*DNSAccount{},
		inCreation: map[string]chan struct{}{},
	}
}

// Get returns a DNSAccount for the given provider, creating it if necessary.
func (m *AccountMap) Get(log logr.Logger, provider *v1alpha1.DNSProvider, props utils.Properties, accountConfig DNSAccountConfig) (*DNSAccount, error) {
	key := client.ObjectKeyFromObject(provider)
	hash := m.Hash(props, provider.Spec.Type, provider.Spec.ProviderConfig)

	account, err := m.getOrCreate(log, provider, props, accountConfig, key, hash)
	if err != nil {
		return nil, err
	}

	m.lock.Lock()
	old := len(account.clients)
	account.clients.Insert(key)
	clientCount := len(account.clients)
	m.lock.Unlock()

	if clientCount != old && old != 0 {
		log.Info("reusing account", "provider", key, "hash", account.Hash(), "clients", clientCount)
	}
	metrics.ReportAccountProviders(provider.Spec.Type, account.Hash(), clientCount)
	return account, nil
}

// getOrCreate returns the existing account for the hash or creates it. Account creation (which may
// perform network I/O, e.g. OpenStack Keystone auth) runs without the lock held, so a slow or hung
// provider cannot block other tenants. Concurrent callers for the same hash wait on a channel that
// the creating goroutine closes when it is done.
func (m *AccountMap) getOrCreate(log logr.Logger, provider *v1alpha1.DNSProvider, props utils.Properties, accountConfig DNSAccountConfig, key client.ObjectKey, hash string) (*DNSAccount, error) {
	for {
		m.lock.Lock()
		if account := m.accounts[hash]; account != nil {
			m.lock.Unlock()
			return account, nil
		}
		if done := m.inCreation[hash]; done != nil {
			m.lock.Unlock()
			<-done // another goroutine is creating this account; wait, then re-check the map
			continue
		}
		done := make(chan struct{})
		m.inCreation[hash] = done
		m.lock.Unlock()

		account, err := m.create(log, provider, props, accountConfig, key, hash)

		m.lock.Lock()
		delete(m.inCreation, hash)
		m.lock.Unlock()
		close(done)
		return account, err
	}
}

// create builds a new DNSAccount and registers it in the map. It must be called by exactly one
// goroutine per hash (serialized via inCreation in getOrCreate).
func (m *AccountMap) create(log logr.Logger, provider *v1alpha1.DNSProvider, props utils.Properties, accountConfig DNSAccountConfig, key client.ObjectKey, hash string) (*DNSAccount, error) {
	candidate := NewDNSAccount(nil, hash, accountConfig)

	rateLimiter, err := NewRateLimiter(accountConfig.RateLimits)
	if err != nil {
		return nil, err
	}

	cfg := DNSHandlerConfig{
		Log:          log,
		Properties:   props,
		Config:       provider.Spec.ProviderConfig,
		GlobalConfig: ptr.Deref(accountConfig.GlobalConfig, config.DNSManagerConfiguration{}),
		Metrics:      candidate,
		RateLimiter:  rateLimiter,
	}
	candidate.handler, err = accountConfig.Factory.Create(provider.Spec.Type, &cfg)
	if err != nil {
		return nil, err
	}

	m.lock.Lock()
	log.Info("creating account", "provider", key, "hash", candidate.Hash())
	m.accounts[hash] = candidate
	m.lock.Unlock()
	return candidate, nil
}

// FindAccountForZone finds the DNSAccount and DNSHostedZone for a given zone ID.
func (m *AccountMap) FindAccountForZone(ctx context.Context, zoneID dns.ZoneID) (*DNSAccount, *DNSHostedZone, error) {
	// Snapshot the matching accounts under the lock, then query them without it: GetZones may
	// perform network I/O (on a cache miss) and must not block other tenants on the global lock.
	m.lock.Lock()
	candidates := make([]*DNSAccount, 0, len(m.accounts))
	for _, account := range m.accounts {
		if account.ProviderType() == zoneID.ProviderType {
			candidates = append(candidates, account)
		}
	}
	m.lock.Unlock()

	var errs []error
	for _, account := range candidates {
		zones, err := account.GetZones(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get zones for account %s: %w", account.Hash(), err))
			continue
		}
		for _, zone := range zones {
			if zone.ZoneID() == zoneID {
				// Account may have been released (handler torn down, removed from the map) while
				// we queried it lock-free; do not return a zombie.
				if !m.isLive(account) {
					break
				}
				return account, &zone, nil
			}
		}
	}
	errs = append([]error{fmt.Errorf("no account found for zone %s", zoneID)}, errs...)
	return nil, nil, errors.Join(errs...)
}

// isLive reports whether the account is still registered in the map (i.e. not released concurrently).
func (m *AccountMap) isLive(a *DNSAccount) bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.accounts[a.hash] == a
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
	// Snapshot the accounts under the lock, then build caches without it: getZoneQueryCache may
	// perform network I/O and must not block other tenants on the global lock.
	m.lock.Lock()
	accounts := make([]*DNSAccount, 0, len(m.accounts))
	for _, account := range m.accounts {
		accounts = append(accounts, account)
	}
	m.lock.Unlock()

	var result []*utils.DNSCache
	for _, account := range accounts {
		if zone := account.GetCachedZone(zoneID); zone != nil {
			zoneInfo := dns.NewZoneInfo(zone.ZoneID(), zone.Domain(), zone.IsPrivate(), zone.Key())
			cache, err := account.getZoneQueryCache(ctx, zoneInfo)
			if err != nil {
				return nil, fmt.Errorf("failed to get DNS cache for zone %s: %w", zoneID, err)
			}
			// Skip accounts released (handler torn down, removed from the map) while we queried
			// them lock-free; do not return a cache for a zombie account.
			if m.isLive(account) {
				result = append(result, cache)
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no DNS caches found for zone %s", zoneID)
	}
	return result, nil
}
