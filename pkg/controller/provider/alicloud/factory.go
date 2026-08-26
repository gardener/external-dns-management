// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package alicloud

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/alicloud/validation"
	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = validation.ProviderType

var rateLimiterDefaults = provider.RateLimiterOptions{
	Enabled: true,
	QPS:     25,
	Burst:   1,
}

var Factory = provider.NewDNSHandlerFactory(NewHandler, validation.NewAdapter(), true).
	SetGenericFactoryOptionDefaults(provider.GenericFactoryOptionDefaults.SetRateLimiterOptions(rateLimiterDefaults))

func init() {
	compound.MustRegister(Factory)
}
