// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"regexp"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/azure/constants"
)

var idRegex = regexp.MustCompile("^[0-9a-fA-F-]+$")

// NewAzureAdapterChecks creates the validation checks for the Azure DNS provider.
func NewAzureAdapterChecks() *provider.DNSHandlerAdapterChecks {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty(constants.PropertySubscriptionID, constants.PropertySubscriptionIDAlias).
		Validators(provider.NoTrailingWhitespaceValidator, provider.RegExValidator(idRegex), provider.MaxLengthValidator(64)))
	checks.Add(provider.RequiredProperty(constants.PropertyClientID, constants.PropertyClientIDAlias).
		Validators(provider.NoTrailingWhitespaceValidator, provider.RegExValidator(idRegex), provider.MaxLengthValidator(64)))
	checks.Add(provider.RequiredProperty(constants.PropertyClientSecret, constants.PropertyClientSecretAlias).
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(64)).
		HideValue())
	checks.Add(provider.RequiredProperty(constants.PropertyTenantID, constants.PropertyTenantIDAlias).
		Validators(provider.NoTrailingWhitespaceValidator, provider.RegExValidator(idRegex), provider.MaxLengthValidator(64)))
	checks.Add(provider.OptionalProperty(constants.PropertyCloud).
		Validators(provider.NoTrailingWhitespaceValidator, provider.PredefinedValuesValidator("AzurePublic", "AzureChina", "AzureGovernment")))
	return checks
}
