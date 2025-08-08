// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/utils"
	miekgdns "github.com/miekg/dns"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

// ProviderType is the type of the RFC2136 DNS provider.
const ProviderType = "rfc2136"

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

// NewAdapter creates a new DNSHandlerAdapter for the RFC2136 DNS provider.
func NewAdapter() provider.DNSHandlerAdapter {
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
	return ProviderType
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
	_, err := FindTsigAlgorithm(value)
	return err
}

// tsigAlgs are the supported TSIG algorithms
var tsigAlgs = []string{miekgdns.HmacSHA1, miekgdns.HmacSHA224, miekgdns.HmacSHA256, miekgdns.HmacSHA384, miekgdns.HmacSHA512}

func FindTsigAlgorithm(alg string) (string, error) {
	if alg == "" {
		return miekgdns.HmacSHA256, nil
	}

	fqdnAlg := miekgdns.Fqdn(alg)
	for _, a := range tsigAlgs {
		if fqdnAlg == a {
			return fqdnAlg, nil
		}
	}
	return "", fmt.Errorf("invalid TSIG secret algorithm: %s (supported: %s)", alg, strings.ReplaceAll(strings.Join(tsigAlgs, ","), ".", ""))
}
