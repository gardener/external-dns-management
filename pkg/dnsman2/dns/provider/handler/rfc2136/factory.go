// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package rfc2136

import (
	"fmt"
	"slices"
	"strings"

	miekgdns "github.com/miekg/dns"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// ProviderType is the type of the RFC2136 DNS provider.
const ProviderType = "rfc2136"

// RegisterTo registers the RFC2136 DNS handler to the given registry.
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

// newAdapter creates a new DNSHandlerAdapter for the RFC2136 DNS provider.
func newAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty(PropertyServer).
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(256)))
	checks.Add(provider.RequiredProperty(PropertyZone).
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(256), zoneValidator))
	checks.Add(provider.RequiredProperty(PropertyTSIGKeyName).
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(256), fqdnValidator))
	checks.Add(provider.RequiredProperty(PropertyTSIGSecret).
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(128)).
		HideValue())
	checks.Add(provider.OptionalProperty(PropertyTSIGSecretAlgorithm).
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
	_, err := findTsigAlgorithm(value)
	return err
}

// tsigAlgs are the supported TSIG algorithms
var tsigAlgs = []string{miekgdns.HmacSHA1, miekgdns.HmacSHA224, miekgdns.HmacSHA256, miekgdns.HmacSHA384, miekgdns.HmacSHA512}

func findTsigAlgorithm(alg string) (string, error) {
	if alg == "" {
		return miekgdns.HmacSHA256, nil
	}

	fqdnAlg := miekgdns.Fqdn(alg)
	if slices.Contains(tsigAlgs, fqdnAlg) {
		return fqdnAlg, nil
	}
	return "", fmt.Errorf("invalid TSIG secret algorithm: %s (supported: %s)", alg, strings.ReplaceAll(strings.Join(tsigAlgs, ","), ".", ""))
}
