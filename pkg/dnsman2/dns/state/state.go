// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"sync"

	"github.com/go-logr/logr"
	"go.uber.org/atomic"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
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
		providers: map[client.ObjectKey]*ProviderState{},
		accounts:  provider.NewAccountMap(),
	}
	if instance.CompareAndSwap(nil, state) {
		return state
	}
	return instance.Load()
}

type State struct {
	lock sync.RWMutex

	providers map[client.ObjectKey]*ProviderState
	accounts  *provider.AccountMap
}

func (s *State) GetOrCreateProviderState(provider *v1alpha1.DNSProvider, config config.DNSProviderControllerConfig) *ProviderState {
	s.lock.Lock()
	defer s.lock.Unlock()
	key := client.ObjectKeyFromObject(provider)
	if state, ok := s.providers[key]; ok {
		return state
	}
	state := NewProviderState(provider)
	state.defaultTTL = ptr.Deref(provider.Spec.DefaultTTL, ptr.Deref(config.DefaultTTL, 360))
	s.providers[key] = state
	return state
}

func (s *State) GetProviderState(providerKey client.ObjectKey) *ProviderState {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.providers[providerKey]
}

func (s *State) GetAccount(log logr.Logger, provider *v1alpha1.DNSProvider, props utils.Properties, config provider.DNSAccountConfig) (*provider.DNSAccount, error) {
	return s.accounts.Get(log, provider, props, config)
}

func (s *State) DeleteProviderState(providerKey client.ObjectKey) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.providers, providerKey)
}

// ClearState clears the singleton state instance (for testing purposes).
func ClearState() {
	instance.Store(nil)
}
