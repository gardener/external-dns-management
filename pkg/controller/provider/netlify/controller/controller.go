// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/netlify"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

func init() {
	provider.DNSController("", netlify.Factory).
		FinalizerDomain("dns.gardener.cloud").
		MustRegister(provider.CONTROLLER_GROUP_DNS_CONTROLLERS)
}
