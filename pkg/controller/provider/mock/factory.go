// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = "mock-inmemory"

var rateLimiterDefaults = provider.RateLimiterOptions{
	Enabled: true,
	QPS:     100,
	Burst:   20,
}

var Factory = provider.NewDNSHandlerFactory(TYPE_CODE, NewHandler).
	SetGenericFactoryOptionDefaults(provider.GenericFactoryOptionDefaults.SetRateLimiterOptions(rateLimiterDefaults))

func init() {
	compound.MustRegister(Factory)
}
