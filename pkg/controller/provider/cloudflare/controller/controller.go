// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/cloudflare"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

func init() {
	provider.DNSController("", cloudflare.Factory).
		FinalizerDomain("dns.gardener.cloud").
		MustRegister(provider.CONTROLLER_GROUP_DNS_CONTROLLERS)
}
