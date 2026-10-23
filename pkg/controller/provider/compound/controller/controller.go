// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

func init() {
	provider.DNSController("", compound.Factory).
		FinalizerDomain("dns.gardener.cloud").
		StringOption(provider.OPT_PROVIDERTYPES, "comma separated list of provider types to enable").
		StringOption(provider.OPT_DISABLE_PROVIDERTYPES, "comma separated list of provider types to disable").
		MustRegister(provider.CONTROLLER_GROUP_DNS_CONTROLLERS)
}
