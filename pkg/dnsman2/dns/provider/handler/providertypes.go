// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"slices"

	"k8s.io/utils/clock"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/alicloud"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/aws"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/azure"
	azureprivate "github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/azure-private"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/cloudflare"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/google"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/local"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/netlify"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/openstack"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/powerdns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/rfc2136"
)

var allProviderTypes = map[string]provider.AddToRegistryFunc{
	alicloud.ProviderType:     alicloud.RegisterTo,
	aws.ProviderType:          aws.RegisterTo,
	azure.ProviderType:        azure.RegisterTo,
	azureprivate.ProviderType: azureprivate.RegisterTo,
	cloudflare.ProviderType:   cloudflare.RegisterTo,
	google.ProviderType:       google.RegisterTo,
	local.ProviderType:        local.RegisterTo,
	netlify.ProviderType:      netlify.RegisterTo,
	openstack.ProviderType:    openstack.RegisterTo,
	rfc2136.ProviderType:      rfc2136.RegisterTo,
	powerdns.ProviderType:     powerdns.RegisterTo,
}

// CreateStandardDNSHandlerFactory creates a DNSHandlerFactory with all standard providers registered,
func CreateStandardDNSHandlerFactory(cfg config.DNSProviderControllerConfig) provider.DNSHandlerFactory {
	registry := provider.NewDNSHandlerRegistry(clock.RealClock{})
	disabledTypes := cfg.DisabledProviderTypes
	enabledTypes := cfg.EnabledProviderTypes
	for providerType, addToRegistry := range allProviderTypes {
		if len(enabledTypes) > 0 && !slices.Contains(enabledTypes, providerType) {
			continue
		}
		if slices.Contains(disabledTypes, providerType) {
			continue
		}
		addToRegistry(registry)
	}
	return registry
}
