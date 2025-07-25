// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	"go.uber.org/atomic"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var (
	instance atomic.Pointer[State]
)

// GetState returns the singleton instance of the State.
func GetState() *State {
	state := instance.Load()
	if state != nil {
		return state
	}

	state = &State{
		providers:      newProviderMap(),
		accounts:       provider.NewAccountMap(),
		dnsNameLocking: newDNSNameLocking(),
	}
	if instance.CompareAndSwap(nil, state) {
		return state
	}
	return instance.Load()
}

// State holds the global state for DNS providers and accounts.
type State struct {
	providers providerMap
	accounts  *provider.AccountMap
	factory   atomic.Pointer[provider.DNSHandlerFactory]

	dnsNameLocking *dnsNameLocking
}

type providerMap struct {
	lock      sync.Mutex
	providers map[client.ObjectKey]*ProviderState
}

func newProviderMap() providerMap {
	return providerMap{
		providers: map[client.ObjectKey]*ProviderState{},
	}
}

// DNSQueryHandler defines an interface for querying DNS records.
type DNSQueryHandler interface {
	// Query performs either a DNS query to the authoritative nameservers or uses the provider API.
	Query(ctx context.Context, setName dns.DNSSetName, rstype dns.RecordType) (dns.Targets, *dns.RoutingPolicy, error)
}

// GetOrCreateProviderState returns the ProviderState for the given DNSProvider, creating it if necessary.
func (s *State) GetOrCreateProviderState(provider *v1alpha1.DNSProvider, config config.DNSProviderControllerConfig) *ProviderState {
	s.providers.lock.Lock()
	defer s.providers.lock.Unlock()
	key := client.ObjectKeyFromObject(provider)
	if state, ok := s.providers.providers[key]; ok {
		return state
	}
	state := NewProviderState(provider)
	state.defaultTTL = ptr.Deref(provider.Spec.DefaultTTL, ptr.Deref(config.DefaultTTL, 360))
	s.providers.providers[key] = state
	return state
}

// GetProviderState returns the ProviderState for the given provider key.
func (s *State) GetProviderState(providerKey client.ObjectKey) *ProviderState {
	s.providers.lock.Lock()
	defer s.providers.lock.Unlock()
	return s.providers.providers[providerKey]
}

// GetAccount retrieves the DNSAccount for the given provider and configuration.
func (s *State) GetAccount(log logr.Logger, provider *v1alpha1.DNSProvider, props utils.Properties, config provider.DNSAccountConfig) (*provider.DNSAccount, error) {
	return s.accounts.Get(log, provider, props, config)
}

// SetDNSHandlerFactory sets the DNSHandlerFactory for the state.
func (s *State) SetDNSHandlerFactory(factory provider.DNSHandlerFactory) {
	s.factory.Store(&factory)
}

// GetDNSHandlerFactory returns the DNSHandlerFactory set in the state.
func (s *State) GetDNSHandlerFactory() provider.DNSHandlerFactory {
	factory := s.factory.Load()
	if factory == nil {
		return nil
	}
	return *factory
}

// FindAccountForZone finds the DNSAccount and DNSHostedZone for the given zone ID.
func (s *State) FindAccountForZone(ctx context.Context, zoneID dns.ZoneID) (*provider.DNSAccount, *provider.DNSHostedZone, error) {
	return s.accounts.FindAccountForZone(ctx, zoneID)
}

// ClearDNSCaches clears the DNS caches for the given zone ID and optional record set keys.
func (s *State) ClearDNSCaches(ctx context.Context, zoneID dns.ZoneID, keys ...utils.RecordSetKey) error {
	caches, err := s.accounts.GetDNSCachesByZone(ctx, zoneID)
	if err != nil {
		return err
	}
	for _, cache := range caches {
		cache.ClearKeys(keys...)
	}
	return nil
}

// DeleteProviderState removes the ProviderState for the given provider key.
func (s *State) DeleteProviderState(providerKey client.ObjectKey) {
	s.providers.lock.Lock()
	defer s.providers.lock.Unlock()
	delete(s.providers.providers, providerKey)
}

// GetDNSQueryHandler returns a DNSQueryHandler for the given zone ID.
func (s *State) GetDNSQueryHandler(ctx context.Context, zoneID dns.ZoneID) (DNSQueryHandler, error) {
	dnscaches, err := s.accounts.GetDNSCachesByZone(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	return newDNSCachesQueryHandler(dnscaches), nil
}

// GetDNSNameLocking returns the dnsNameLocking instance used for managing DNS name locks.
func (s *State) GetDNSNameLocking() *dnsNameLocking {
	return s.dnsNameLocking
}

// ClearState clears the singleton state instance (for testing purposes).
func ClearState() {
	instance.Store(nil)
}

type dnsCachesQueryHandler struct {
	dnscaches []*utils.DNSCache
}

func newDNSCachesQueryHandler(dnscaches []*utils.DNSCache) DNSQueryHandler {
	return &dnsCachesQueryHandler{dnscaches: dnscaches}
}

func (h *dnsCachesQueryHandler) Query(ctx context.Context, setName dns.DNSSetName, rstype dns.RecordType) (dns.Targets, *dns.RoutingPolicy, error) {
	var err error
	for _, cache := range h.dnscaches {
		rs, err := cache.Get(ctx, setName, rstype)
		if err != nil {
			continue
		}
		var (
			targets       dns.Targets
			routingPolicy *dns.RoutingPolicy
		)
		if rs != nil {
			for _, record := range rs.Records {
				targets = append(targets, dns.NewTarget(rstype, record.Value, rs.TTL))
			}
			routingPolicy = rs.RoutingPolicy
		}
		return targets, routingPolicy, err
	}
	return nil, nil, err
}
