// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"regexp"

	"github.com/gardener/controller-manager-library/pkg/utils"
	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const (
	ProviderTypeAzureDNS        = "azure-dns"
	ProviderTypeAzurePrivateDNS = "azure-private-dns"
)

type adapter struct {
	providerType string
	checks       *provider.DNSHandlerAdapterChecks
}

var idRegex = regexp.MustCompile("^[0-9a-fA-F-]+$")

// NewAdapter creates a new Azure DNS handler adapter with the required checks for credentials.
func NewAdapter(providerType string) provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.SetDisjunctPropertySets([]string{"AZURE_SUBSCRIPTION_ID", "AZURE_CLIENT_SECRET", "AZURE_CLIENT_ID", "AZURE_TENANT_ID"},
		[]string{securityv1alpha1constants.DataKeyToken, securityv1alpha1constants.DataKeyConfig, securityv1alpha1constants.LabelWorkloadIdentityProvider})
	checks.Add(provider.OptionalProperty("AZURE_SUBSCRIPTION_ID", "subscriptionID").
		Validators(provider.NoTrailingWhitespaceValidator, provider.RegExValidator(idRegex), provider.MaxLengthValidator(64)))
	checks.Add(provider.OptionalProperty("AZURE_CLIENT_SECRET", "clientSecret").
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(64)).
		HideValue())
	checks.Add(provider.OptionalProperty("AZURE_CLIENT_ID", "clientID").
		Validators(provider.NoTrailingWhitespaceValidator, provider.RegExValidator(idRegex), provider.MaxLengthValidator(64)))
	checks.Add(provider.OptionalProperty("AZURE_TENANT_ID", "tenantID").
		Validators(provider.NoTrailingWhitespaceValidator, provider.RegExValidator(idRegex), provider.MaxLengthValidator(64)))
	checks.Add(provider.OptionalProperty("AZURE_CLOUD").
		Validators(provider.NoTrailingWhitespaceValidator, provider.PredefinedValuesValidator("AzurePublic", "AzureChina", "AzureGovernment")))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.DataKeyToken).
		Validators(provider.MaxLengthValidator(4096)))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.DataKeyConfig).
		Validators(provider.MaxLengthValidator(4096)))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.LabelWorkloadIdentityProvider).
		Validators(provider.ExpectedValueValidator("azure")))
	return &adapter{providerType: providerType, checks: checks}
}

func (a *adapter) ProviderType() string {
	return a.providerType
}

func (a *adapter) ValidateCredentialsAndProviderConfig(properties utils.Properties, config *runtime.RawExtension) error {
	if config != nil && len(config.Raw) > 0 {
		return fmt.Errorf("provider config not supported for %s provider", a.ProviderType())
	}
	return a.checks.ValidateProperties(a.ProviderType(), properties)
}
