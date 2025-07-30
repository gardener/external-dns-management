// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package powerdns

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = "powerdns"

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
	checks.Add(provider.RequiredProperty("Server", "server").
		Validators(provider.NoTrailingWhitespaceValidator, provider.PrintableValidator, provider.URLValidator("https", "http"), provider.MaxLengthValidator(256)))
	checks.Add(provider.RequiredProperty("ApiKey", "apiKey").
		Validators(provider.NoTrailingWhitespaceValidator, provider.PrintableValidator, provider.MaxLengthValidator(8192)).
		HideValue())
	checks.Add(provider.OptionalProperty("VirtualHost", "virtualHost").
		Validators(provider.NoTrailingWhitespaceValidator, provider.PrintableValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("InsecureSkipVerify", "insecureSkipVerify").
		Validators(provider.BoolValidator))
	checks.Add(provider.OptionalProperty("TrustedCaCert", "trustedCaCert").
		Validators(provider.CACertValidator).
		HideValue())
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
