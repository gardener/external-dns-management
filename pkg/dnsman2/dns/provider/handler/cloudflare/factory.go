// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cloudflare

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// ProviderType is the type code for the Cloudflare DNS provider.
const ProviderType = "cloudflare-dns"

// RegisterTo registers the Cloudflare DNS provider to the given registry.
func RegisterTo(registry *provider.DNSHandlerRegistry) {
	registry.Register(
		ProviderType,
		NewHandler,
		newAdapter(),
		&config.RateLimiterOptions{
			Enabled: true,
			QPS:     50,
			Burst:   10,
		},
		nil,
	)
}

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

// NewAdapter creates a new DNSHandlerAdapter for the Cloudflare DNS provider.
func newAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty("CLOUDFLARE_API_TOKEN").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(100)).
		HideValue())
	return &adapter{checks: checks}
}

func (a *adapter) ProviderType() string {
	return ProviderType
}

func (a *adapter) ValidateCredentialsAndProviderConfig(properties utils.Properties, config *runtime.RawExtension) error {
	if config != nil && len(config.Raw) > 0 {
		return fmt.Errorf("provider config not supported for %s provider", a.ProviderType())
	}
	return a.checks.ValidateProperties(a.ProviderType(), properties)
}
