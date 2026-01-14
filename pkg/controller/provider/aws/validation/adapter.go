// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/gardener/controller-manager-library/pkg/utils"
	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/controller/provider/aws/config"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

// ProviderType is the type identifier for the AWS Route53 DNS provider.
const ProviderType = "aws-route53"

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

var (
	regionRegex = regexp.MustCompile("^[a-z0-9-]*$") // empty string is explicitly allowed to match the default region
	arnRegex    = regexp.MustCompile(`^arn:aws[a-zA-Z-]*:iam::[0-9]{12}:role\/[\w+=,.@\-_/]+$`)
)

// NewAdapter creates a new DNSHandlerAdapter for the AWS Route53 provider.
func NewAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.SetDisjunctPropertySets([]string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"},
		[]string{"AWS_USE_CREDENTIALS_CHAIN"},
		[]string{securityv1alpha1constants.DataKeyToken, securityv1alpha1constants.DataKeyConfig, securityv1alpha1constants.LabelWorkloadIdentityProvider})
	checks.Add(provider.OptionalProperty("AWS_ACCESS_KEY_ID", "accessKeyID").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericValidator, provider.MaxLengthValidator(128)))
	checks.Add(provider.OptionalProperty("AWS_SECRET_ACCESS_KEY", "secretAccessKey").
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(128)).
		HideValue())
	checks.Add(provider.OptionalProperty("AWS_REGION", "region").
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(32), provider.RegExValidator(regionRegex)).
		AllowEmptyValue())
	checks.Add(provider.OptionalProperty("AWS_USE_CREDENTIALS_CHAIN").
		Validators(provider.NoTrailingWhitespaceValidator, provider.BoolValidator))
	checks.Add(provider.OptionalProperty("AWS_SESSION_TOKEN", "sessionToken").
		Validators(provider.MaxLengthValidator(512)).
		HideValue())
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.DataKeyToken).
		Validators(provider.MaxLengthValidator(4096)))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.DataKeyConfig).
		Validators(provider.MaxLengthValidator(4096)))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.LabelWorkloadIdentityProvider).
		Validators(provider.ExpectedValueValidator("aws")))
	checks.Add(provider.OptionalProperty(dns.RoleARN).
		Validators(provider.NoTrailingWhitespaceValidator, provider.RegExValidator(arnRegex)))
	return &adapter{checks: checks}
}

func (a *adapter) ProviderType() string {
	return ProviderType
}

func (a *adapter) ValidateCredentialsAndProviderConfig(properties utils.Properties, raw *runtime.RawExtension) error {
	if raw != nil && len(raw.Raw) > 0 {
		var cfg config.AWSConfig
		err := json.Unmarshal(raw.Raw, &cfg)
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
