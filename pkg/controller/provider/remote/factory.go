// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = "remote"

var rateLimiterDefaults = provider.RateLimiterOptions{
	Enabled: true,
	QPS:     9,
	Burst:   10,
}

var advancedDefaults = provider.AdvancedOptions{
	BatchSize:  50,
	MaxRetries: 7,
}

var Factory = provider.NewDNSHandlerFactory(TYPE_CODE, NewHandler).
	SetGenericFactoryOptionDefaults(provider.GenericFactoryOptionDefaults.
		SetRateLimiterOptions(rateLimiterDefaults).SetAdvancedOptions(advancedDefaults))

func init() {
	compound.MustRegister(Factory)
}
