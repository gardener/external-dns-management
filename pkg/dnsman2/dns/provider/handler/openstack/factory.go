// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// ProviderType is the type identifier for the OpenStack Designate DNS provider.
const ProviderType = "openstack-designate"

// RegisterTo registers the OpenStack Designate DNS handler to the given registry.
func RegisterTo(registry *provider.DNSHandlerRegistry) {
	registry.Register(
		ProviderType,
		NewHandler,
		newAdapter(),
		&config.RateLimiterOptions{
			Enabled: true,
			QPS:     100,
			Burst:   20,
		},
		nil,
	)
}

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

// newAdapter creates a new DNSHandlerAdapter for the OpenStack Designate provider.
func newAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty("OS_AUTH_URL", "authURL").
		Validators(provider.NoTrailingWhitespaceValidator, provider.URLValidator("http", "https"), provider.MaxLengthValidator(256)))
	checks.Add(provider.OptionalProperty("OS_APPLICATION_CREDENTIAL_ID", "applicationCredentialID").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("OS_APPLICATION_CREDENTIAL_NAME", "applicationCredentialName").
		Validators(provider.NoTrailingWhitespaceValidator, provider.PrintableValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("OS_APPLICATION_CREDENTIAL_SECRET", "applicationCredentialSecret").
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(128)).
		HideValue())
	checks.Add(provider.OptionalProperty("OS_USERNAME", "username").
		Validators(provider.NoTrailingWhitespaceValidator, provider.PrintableValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("OS_PASSWORD", "password").
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(128), provider.NoTrailingNewlineValidator).
		HideValue())
	checks.Add(provider.OptionalProperty("OS_DOMAIN_NAME", "domainName").
		Validators(provider.NoTrailingWhitespaceValidator, provider.PrintableValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("OS_DOMAIN_ID", "domainID").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(128)))
	// tenantName must not be longer than 64 characters, see https://docs.openstack.org/api-ref/identity/v3/?expanded=show-project-details-detail
	checks.Add(provider.OptionalProperty("OS_PROJECT_NAME", "tenantName").
		Validators(provider.NoTrailingWhitespaceValidator, provider.PrintableValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("OS_PROJECT_ID", "tenantID").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("OS_USER_DOMAIN_NAME", "userDomainName").
		Validators(provider.NoTrailingWhitespaceValidator, provider.PrintableValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("OS_USER_DOMAIN_ID", "userDomainID").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("OS_REGION_NAME").
		Validators(provider.NoTrailingWhitespaceValidator, provider.PrintableValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("CACERT", "caCert").Validators(provider.CACertValidator).HideValue())
	checks.Add(provider.OptionalProperty("CLIENTCERT", "clientCert").Validators(provider.PEMValidator).HideValue())
	checks.Add(provider.OptionalProperty("CLIENTKEY", "clientKey").Validators(provider.PEMValidator).HideValue())
	checks.Add(provider.OptionalProperty("INSECURE", "insecure").Validators(provider.BoolValidator))
	checks.Add(provider.OptionalProperty("OS_IDENTITY_API_VERSION").Validators(provider.IntValidator(2, 100))) // not used, but some users might want to set it
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
