// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package rfc2136

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/utils"
	miekgdns "github.com/miekg/dns"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = "rfc2136"

var rateLimiterDefaults = provider.RateLimiterOptions{
	Enabled: true,
	QPS:     50,
	Burst:   10,
}

var Factory = provider.NewDNSHandlerFactory(TYPE_CODE, NewHandler, newAdapter()).
	SetGenericFactoryOptionDefaults(provider.GenericFactoryOptionDefaults.SetRateLimiterOptions(rateLimiterDefaults))

func init() {
	compound.MustRegister(Factory)
}

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

func newAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty("Server").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(256)))
	checks.Add(provider.RequiredProperty("Zone").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(256), zoneValidator))
	checks.Add(provider.RequiredProperty("TSIGKeyName").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(256), fqdnValidator))
	checks.Add(provider.RequiredProperty("TSIGSecret").
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(128)).
		HideValue())
	checks.Add(provider.OptionalProperty("TSIGSecretAlgorithm").
		Validators(algorithmValidator))
	return &adapter{checks: checks}
}

func (a *adapter) ProviderType() string {
	return TYPE_CODE
}

func (a *adapter) ValidateCredentialsAndProviderConfig(properties utils.Properties, config *runtime.RawExtension) error {
	if config != nil && len(config.Raw) > 0 {
		return fmt.Errorf("provider config not supported for %s provider", a.ProviderType())
	}
	return a.checks.ValidateProperties(a.ProviderType(), properties)
}

func zoneValidator(value string) error {
	if value != miekgdns.CanonicalName(value) {
		return fmt.Errorf("zone must be given in canonical form: '%s' instead of '%s'", miekgdns.CanonicalName(value), value)
	}
	return nil
}

func fqdnValidator(value string) error {
	if value != miekgdns.Fqdn(value) {
		return fmt.Errorf("TSIGKeyName must end with '.'")
	}
	return nil
}

func algorithmValidator(value string) error {
	_, err := findTsigAlgorithm(value)
	return err
}
