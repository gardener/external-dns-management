// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

const ProviderType = "mock-inmemory"

func RegisterTo(registry *provider.DNSHandlerRegistry) {
	registry.Register(ProviderType, NewHandler)
}
