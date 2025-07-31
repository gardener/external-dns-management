// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = "aws-route53"

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

var regionRegex = regexp.MustCompile("^[a-z0-9-]*$") // empty string is explicitly allowed to match the default region

func newAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.OptionalProperty("AWS_ACCESS_KEY_ID", "accessKeyID").
		RequiredIfUnset([]string{"AWS_USE_CREDENTIALS_CHAIN"}).
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("AWS_SECRET_ACCESS_KEY", "secretAccessKey").
		RequiredIfUnset([]string{"AWS_USE_CREDENTIALS_CHAIN"}).
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(128)).
		HideValue())
	checks.Add(provider.OptionalProperty("AWS_REGION", "region").
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(32), provider.RegExValidator(regionRegex)).
		AllowEmptyValue())
	checks.Add(provider.OptionalProperty("AWS_USE_CREDENTIALS_CHAIN").
		RequiredIfUnset([]string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}).
		Validators(provider.NoTrailingWhitespaceValidator, provider.BoolValidator))
	checks.Add(provider.OptionalProperty("AWS_SESSION_TOKEN").
		Validators(provider.MaxLengthValidator(512)).
		HideValue())
	return &adapter{checks: checks}
}

func (a *adapter) ProviderType() string {
	return TYPE_CODE
}

func (a *adapter) ValidateCredentialsAndProviderConfig(properties utils.Properties, config *runtime.RawExtension) error {
	if config != nil && len(config.Raw) > 0 {
		var cfg AWSConfig
		err := json.Unmarshal(config.Raw, &cfg)
		if err != nil {
			return fmt.Errorf("unmarshal providerConfig failed with: %w", err)
		}
		if cfg.BatchSize < 1 || cfg.BatchSize > 50 {
			return fmt.Errorf("invalid batch size %d, must be between 1 and 50", cfg.BatchSize)
		}
		return nil
	}
	return a.checks.ValidateProperties(a.ProviderType(), properties)
}
