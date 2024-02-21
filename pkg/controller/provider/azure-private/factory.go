// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package azureprivate

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = "azure-private-dns"

var rateLimiterDefaults = provider.RateLimiterOptions{
	Enabled: true,
	QPS:     50,
	Burst:   10,
}

var Factory = provider.NewDNSHandlerFactory(TYPE_CODE, NewHandler).
	SetGenericFactoryOptionDefaults(provider.GenericFactoryOptionDefaults.SetRateLimiterOptions(rateLimiterDefaults))

func init() {
	compound.MustRegister(Factory)
}
