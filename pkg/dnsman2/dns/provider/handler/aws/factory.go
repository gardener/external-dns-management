// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

const ProviderType = "aws-route53"

// TODO (Martin Weindel) how to deal with these settings?
/*
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
*/

func RegisterTo(registry *provider.DNSHandlerRegistry) {
	registry.Register(ProviderType, NewHandler)
}
