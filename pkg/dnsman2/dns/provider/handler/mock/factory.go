// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

// ProviderType is the identifier for the mock in-memory DNS provider.
const ProviderType = "mock-inmemory"

// RegisterTo registers the mock DNS handler to the given registry.
func RegisterTo(registry *provider.DNSHandlerRegistry) {
	registry.Register(ProviderType, NewHandler)
}
