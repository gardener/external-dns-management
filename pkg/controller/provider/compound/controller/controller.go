// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

func init() {
	provider.DNSController("", compound.Factory).
		FinalizerDomain("dns.gardener.cloud").StringOption(provider.OPT_PROVIDERTYPES, "comma separated list of provider types to enable").
		MustRegister(provider.CONTROLLER_GROUP_DNS_CONTROLLERS)
}
