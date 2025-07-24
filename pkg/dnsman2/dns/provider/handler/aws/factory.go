// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"encoding/json"
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/aws/mapping"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// ProviderType is the type identifier for the AWS Route53 DNS handler.
const ProviderType = "aws-route53"

const (
	defaultBatchSize  = 50
	defaultMaxRetries = 7
)

// RegisterTo registers the AWS Route53 DNS handler to the given registry.
func RegisterTo(registry *provider.DNSHandlerRegistry) {
	registry.Register(
		ProviderType,
		NewHandler,
		newAdapter(),
		&config.RateLimiterOptions{
			Enabled: true,
			QPS:     9,
			Burst:   10,
		},
		&targetsMapper{},
	)
}

type targetsMapper struct{}

func (m *targetsMapper) MapTargets(targets []dns.Target) []dns.Target {
	return mapping.MapTargets(targets)
}

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

var regionRegex = regexp.MustCompile("^[a-z0-9-]*$") // empty string is explicitly allowed to match the default region

func newAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty("AWS_ACCESS_KEY_ID", "accessKeyID").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.RequiredProperty("AWS_SECRET_ACCESS_KEY", "secretAccessKey").
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(128)).
		HideValue())
	checks.Add(provider.OptionalProperty("AWS_REGION", "region").
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(32), provider.RegExValidator(regionRegex)).
		AllowEmptyValue())
	checks.Add(provider.OptionalProperty("AWS_USE_CREDENTIALS_CHAIN").
		Validators(provider.NoTrailingWhitespaceValidator, provider.BoolValidator))
	checks.Add(provider.OptionalProperty("AWS_SESSION_TOKEN").
		Validators(provider.MaxLengthValidator(512)).
		HideValue())
	return &adapter{checks: checks}
}

func (a *adapter) ProviderType() string {
	return ProviderType
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
