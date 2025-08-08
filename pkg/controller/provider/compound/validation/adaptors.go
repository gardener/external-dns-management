// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	alicloudvalidation "github.com/gardener/external-dns-management/pkg/controller/provider/alicloud/validation"
	awsvalidation "github.com/gardener/external-dns-management/pkg/controller/provider/aws/validation"
	azurevalidation "github.com/gardener/external-dns-management/pkg/controller/provider/azure/validation"
	cloudflarevalidation "github.com/gardener/external-dns-management/pkg/controller/provider/cloudflare/validation"
	googlevalidation "github.com/gardener/external-dns-management/pkg/controller/provider/google/validation"
	infobloxvalidation "github.com/gardener/external-dns-management/pkg/controller/provider/infoblox/validation"
	netlifyvalidation "github.com/gardener/external-dns-management/pkg/controller/provider/netlify/validation"
	openstackvalidation "github.com/gardener/external-dns-management/pkg/controller/provider/openstack/validation"
	powerdnsvalidation "github.com/gardener/external-dns-management/pkg/controller/provider/powerdns/validation"
	remotevalidation "github.com/gardener/external-dns-management/pkg/controller/provider/remote/validation"
	rfc2136validation "github.com/gardener/external-dns-management/pkg/controller/provider/rfc2136/validation"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

var adaptors map[string]provider.DNSHandlerAdapter

func init() {
	adaptors = make(map[string]provider.DNSHandlerAdapter)
	for _, adaptor := range []provider.DNSHandlerAdapter{
		alicloudvalidation.NewAdapter(),
		awsvalidation.NewAdapter(),
		azurevalidation.NewAdapter(azurevalidation.ProviderTypeAzureDNS),
		azurevalidation.NewAdapter(azurevalidation.ProviderTypeAzurePrivateDNS),
		cloudflarevalidation.NewAdapter(),
		googlevalidation.NewAdapter(),
		infobloxvalidation.NewAdapter(),
		netlifyvalidation.NewAdapter(),
		openstackvalidation.NewAdapter(),
		powerdnsvalidation.NewAdapter(),
		remotevalidation.NewAdapter(),
		rfc2136validation.NewAdapter(),
	} {
		registerAdaptor(adaptor)
	}
}

func registerAdaptor(adaptor provider.DNSHandlerAdapter) {
	if _, exists := adaptors[adaptor.ProviderType()]; exists {
		panic("dns handler adaptor already registered for code: " + adaptor.ProviderType())
	}
	adaptors[adaptor.ProviderType()] = adaptor
}

// GetAdaptor returns the DNSHandlerAdapter for the given provider type.
// Used for validation by the shoot-dns-service extension.
func GetAdaptor(providerType string) provider.DNSHandlerAdapter {
	return adaptors[providerType]
}
