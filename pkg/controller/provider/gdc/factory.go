// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package gdc implements the Google Distributed Cloud (GDC) air-gapped DNS provider.
package gdc

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/validation"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

// TYPE_CODE is the provider type identifier for the GDC DNS provider ("gdch-dns").
const TYPE_CODE = validation.ProviderType

var rateLimiterDefaults = provider.RateLimiterOptions{
	Enabled: true,
	QPS:     100,
	Burst:   20,
}

// Factory is the DNSHandlerFactory that registers the GDC DNS provider handler and credential adapter.
var Factory = provider.NewDNSHandlerFactory(NewHandler, validation.NewAdapter()).
	SetGenericFactoryOptionDefaults(provider.GenericFactoryOptionDefaults.SetRateLimiterOptions(rateLimiterDefaults))

func init() {
	compound.MustRegister(Factory)
}
