// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"sync"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/selection"
)

// ProviderState holds state information for a DNS provider, including account and selection details.
type ProviderState struct {
	lock sync.Mutex

	lastVersion *v1alpha1.DNSProvider

	account *provider.DNSAccount
	// TODO(marc1404): Use this field or remove it
	// nolint:unused
	valid bool

	defaultTTL int64
	selection  selection.SelectionResult
}

// NewProviderState creates a new ProviderState for the given DNSProvider.
func NewProviderState(provider *v1alpha1.DNSProvider) *ProviderState {
	return &ProviderState{
		lastVersion: provider.DeepCopy(),
	}
}

// GetAccount returns the DNSAccount associated with the ProviderState.
func (s *ProviderState) GetAccount() *provider.DNSAccount {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.account
}

// SetAccount sets the DNSAccount for the ProviderState.
func (s *ProviderState) SetAccount(account *provider.DNSAccount) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.account = account
}

// GetSelection returns the SelectionResult associated with the ProviderState.
func (s *ProviderState) GetSelection() selection.SelectionResult {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.selection
}

// SetSelection sets the SelectionResult for the ProviderState.
func (s *ProviderState) SetSelection(selection selection.SelectionResult) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.selection = selection
}

// GetDefaultTTL returns the default TTL value for the ProviderState.
func (s *ProviderState) GetDefaultTTL() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.defaultTTL
}

// IsValid returns whether the ProviderState is valid.
// TODO(MartinWeindel): needed?
func (s *ProviderState) IsValid() bool {
	// TODO(MartinWeindel) needed?
	return true
}
