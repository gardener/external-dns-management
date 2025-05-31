// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"go.uber.org/atomic"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/mock"
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
		providers: newProviderMap(),
		accounts:  provider.NewAccountMap(),
	}
	if instance.CompareAndSwap(nil, state) {
		return state
	}
	return instance.Load()
}

type State struct {
	providers providerMap
	accounts  *provider.AccountMap
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

type DNSQueryHandler interface {
	// Query performs either a DNS query to the authoritative nameservers or uses the provider API.
	Query(ctx context.Context, fqdn, setIdentifier string, rstype dns.RecordType) (dns.Targets, *dns.RoutingPolicy, error)
}

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

func (s *State) GetProviderState(providerKey client.ObjectKey) *ProviderState {
	s.providers.lock.Lock()
	defer s.providers.lock.Unlock()
	return s.providers.providers[providerKey]
}

func (s *State) GetAccount(log logr.Logger, provider *v1alpha1.DNSProvider, props utils.Properties, config provider.DNSAccountConfig) (*provider.DNSAccount, error) {
	return s.accounts.Get(log, provider, props, config)
}

func (s *State) FindAccountForZone(ctx context.Context, zoneID dns.ZoneID) (*provider.DNSAccount, *provider.DNSHostedZone, error) {
	return s.accounts.FindAccountForZone(ctx, zoneID)
}

func (s *State) DeleteProviderState(providerKey client.ObjectKey) {
	s.providers.lock.Lock()
	defer s.providers.lock.Unlock()
	delete(s.providers.providers, providerKey)
}

func (s *State) GetDNSQueryHandler(zoneID dns.ZoneID) (DNSQueryHandler, error) {
	if zoneID.ProviderType == mock.ProviderType {
		return newMockDNSQueryHandler(zoneID)
	}

	dnscaches, err := s.accounts.GetDNSCachesByZone(zoneID)
	if err != nil {
		return nil, err
	}
	return newDNSCachesQueryHandler(dnscaches), nil
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

func (h *dnsCachesQueryHandler) Query(ctx context.Context, fqdn, setIdentifier string, rstype dns.RecordType) (dns.Targets, *dns.RoutingPolicy, error) {
	if setIdentifier != "" {
		return nil, nil, fmt.Errorf("setIdentifier is not supported by DNSCachesQueryHandler") // TODO(MartinWeindel): support setIdentifier
	}
	var err error
	for _, cache := range h.dnscaches {
		err = nil
		result := cache.Get(ctx, fqdn, rstype)
		if result.Err != nil {
			err = result.Err
			continue
		}
		var targets dns.Targets
		for _, record := range result.Records {
			targets = append(targets, dns.NewTarget(rstype, record.Value, int64(result.TTL)))
		}
		return targets, nil, err
	}
	return nil, nil, err
}

type mockDNSQueryHandler struct {
	inMemory *mock.InMemory
	zoneID   dns.ZoneID
}

func newMockDNSQueryHandler(zoneID dns.ZoneID) (DNSQueryHandler, error) {
	inMemory := mock.GetInMemoryMockByZoneID(zoneID)
	if inMemory == nil {
		return nil, fmt.Errorf("no mock handler found for zoneID %s", zoneID)
	}
	return &mockDNSQueryHandler{inMemory: inMemory, zoneID: zoneID}, nil
}

func (h *mockDNSQueryHandler) Query(_ context.Context, fqdn, setIdentifier string, rstype dns.RecordType) (dns.Targets, *dns.RoutingPolicy, error) {
	recordSet := h.inMemory.GetRecordset(h.zoneID, dns.DNSSetName{DNSName: fqdn, SetIdentifier: setIdentifier}, rstype)
	if recordSet == nil {
		return nil, nil, nil
	}

	var targets dns.Targets
	var ttl int64
	if !recordSet.IsTTLIgnored() {
		ttl = recordSet.TTL
	}
	for _, record := range recordSet.Records {
		targets = append(targets, dns.NewTarget(rstype, record.Value, ttl))
	}
	return targets, nil, nil
}
