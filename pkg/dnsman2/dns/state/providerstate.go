// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/selection"
)

type ProviderState struct {
	lastVersion *v1alpha1.DNSProvider

	account *provider.DNSAccount
	// TODO(marc1404): Use this field or remove it
	// nolint:unused
	valid bool

	defaultTTL int64
	selection  selection.SelectionResult
}

func NewProviderState(provider *v1alpha1.DNSProvider) *ProviderState {
	return &ProviderState{
		lastVersion: provider.DeepCopy(),
	}
}

func (s *ProviderState) GetAccount() *provider.DNSAccount {
	return s.account
}

func (s *ProviderState) SetAccount(account *provider.DNSAccount) {
	s.account = account
}

func (s *ProviderState) GetSelection() selection.SelectionResult {
	return s.selection
}

func (s *ProviderState) SetSelection(selection selection.SelectionResult) {
	s.selection = selection
}

func (s *ProviderState) GetDefaultTTL() int64 {
	return s.defaultTTL
}
