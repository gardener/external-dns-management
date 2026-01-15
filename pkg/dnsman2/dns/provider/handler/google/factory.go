// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package google

import (
	"fmt"

	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// ProviderType is the type identifier for the Google Cloud DNS handler.
const ProviderType = "google-clouddns"

// RegisterTo registers the Google Cloud DNS handler to the given registry.
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

func newAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.SetDisjunctPropertySets([]string{"serviceaccount.json"},
		[]string{securityv1alpha1constants.DataKeyToken, securityv1alpha1constants.DataKeyConfig, securityv1alpha1constants.LabelWorkloadIdentityProvider})
	checks.Add(provider.OptionalProperty("serviceaccount.json").Validators(func(value string) error {
		_, err := validateServiceAccountJSON([]byte(value))
		return err
	}).HideValue())
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.DataKeyToken).
		Validators(provider.MaxLengthValidator(4096)))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.DataKeyConfig).
		Validators(provider.MaxLengthValidator(4096)))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.LabelWorkloadIdentityProvider).
		Validators(provider.ExpectedValueValidator("gcp")))
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
