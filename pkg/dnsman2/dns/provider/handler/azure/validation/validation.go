// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"regexp"

	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/azure/constants"
)

var idRegex = regexp.MustCompile("^[0-9a-fA-F-]+$")

// NewAzureAdapterChecks creates the validation checks for the Azure DNS provider.
func NewAzureAdapterChecks() *provider.DNSHandlerAdapterChecks {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.SetDisjunctPropertySets([]string{constants.PropertySubscriptionID, constants.PropertyClientID, constants.PropertyClientSecret, constants.PropertyTenantID},
		[]string{securityv1alpha1constants.DataKeyToken, securityv1alpha1constants.DataKeyConfig, securityv1alpha1constants.LabelWorkloadIdentityProvider})
	checks.Add(provider.OptionalProperty(constants.PropertySubscriptionID, constants.PropertySubscriptionIDAlias).
		Validators(provider.NoTrailingWhitespaceValidator, provider.RegExValidator(idRegex), provider.MaxLengthValidator(64)))
	checks.Add(provider.OptionalProperty(constants.PropertyClientID, constants.PropertyClientIDAlias).
		Validators(provider.NoTrailingWhitespaceValidator, provider.RegExValidator(idRegex), provider.MaxLengthValidator(64)))
	checks.Add(provider.OptionalProperty(constants.PropertyClientSecret, constants.PropertyClientSecretAlias).
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(64)).
		HideValue())
	checks.Add(provider.OptionalProperty(constants.PropertyTenantID, constants.PropertyTenantIDAlias).
		Validators(provider.NoTrailingWhitespaceValidator, provider.RegExValidator(idRegex), provider.MaxLengthValidator(64)))
	checks.Add(provider.OptionalProperty(constants.PropertyCloud).
		Validators(provider.NoTrailingWhitespaceValidator, provider.PredefinedValuesValidator("AzurePublic", "AzureChina", "AzureGovernment")))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.DataKeyToken).
		Validators(provider.MaxLengthValidator(4096)))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.DataKeyConfig).
		Validators(provider.MaxLengthValidator(4096)))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.LabelWorkloadIdentityProvider).
		Validators(provider.ExpectedValueValidator("azure")))

	return checks
}
