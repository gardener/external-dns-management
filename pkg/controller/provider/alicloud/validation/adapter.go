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

const ProviderType = "alicloud-dns"

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

// NewAdapter creates a new DNSHandlerAdapter for the Alicloud DNS provider.
func NewAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty("ACCESS_KEY_ID", "accessKeyID").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericValidator, provider.MaxLengthValidator(64)))
	checks.Add(provider.RequiredProperty("ACCESS_KEY_SECRET", "accessKeySecret").
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(64)).
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
