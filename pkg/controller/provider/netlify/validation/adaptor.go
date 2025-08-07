// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

// Provider type code for Netlify DNS provider
const ProviderType = "netlify-dns"

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

// NewAdapter creates a new DNSHandlerAdapter for the Netlify DNS provider
func NewAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty("NETLIFY_AUTH_TOKEN", "NETLIFY_API_TOKEN").
		Validators(provider.NoTrailingWhitespaceValidator, provider.Base64CharactersValidator, provider.MaxLengthValidator(64)).
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
