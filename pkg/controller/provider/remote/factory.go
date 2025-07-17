// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = "remote"

var rateLimiterDefaults = provider.RateLimiterOptions{
	Enabled: true,
	QPS:     9,
	Burst:   10,
}

var advancedDefaults = provider.AdvancedOptions{
	BatchSize:  50,
	MaxRetries: 7,
}

var Factory = provider.NewDNSHandlerFactory(TYPE_CODE, NewHandler, newAdapter()).
	SetGenericFactoryOptionDefaults(provider.GenericFactoryOptionDefaults.
		SetRateLimiterOptions(rateLimiterDefaults).SetAdvancedOptions(advancedDefaults))

func init() {
	compound.MustRegister(Factory)
}

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

func newAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty("REMOTE_ENDPOINT", "remoteEndpoint").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(256)))
	checks.Add(provider.RequiredProperty("CLIENT_CERT", "tls.crt").
		Validators(provider.PEMValidator).
		HideValue())
	checks.Add(provider.RequiredProperty("CLIENT_KEY", "tls.key").
		Validators(provider.PEMValidator).
		HideValue())
	checks.Add(provider.RequiredProperty("NAMESPACE", "namespace").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(64)))
	checks.Add(provider.OptionalProperty("SERVER_CA_CERT", "ca.crt").
		Validators(provider.CACertValidator).HideValue())
	checks.Add(provider.OptionalProperty("OVERRIDE_SERVER_NAME", "overrideServerName").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(64)))
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
