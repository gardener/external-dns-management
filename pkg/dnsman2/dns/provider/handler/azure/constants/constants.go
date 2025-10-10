// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package constants

const (
	// PropertySubscriptionID is the property name for the Azure subscription ID.
	PropertySubscriptionID = "AZURE_SUBSCRIPTION_ID"
	// PropertySubscriptionIDAlias is an alias for PropertySubscriptionID.
	PropertySubscriptionIDAlias = "subscriptionID"

	// PropertyClientID is the property name for the Azure client ID.
	PropertyClientID = "AZURE_CLIENT_ID"
	// PropertyClientIDAlias is an alias for PropertyClientID.
	PropertyClientIDAlias = "clientID"

	// PropertyClientSecret is the property name for the Azure client secret.
	PropertyClientSecret = "AZURE_CLIENT_SECRET" // #nosec G101 -- false positive
	// PropertyClientSecretAlias is an alias for PropertyClientSecret.
	PropertyClientSecretAlias = "clientSecret"

	// PropertyTenantID is the property name for the Azure tenant ID.
	PropertyTenantID = "AZURE_TENANT_ID"
	// PropertyTenantIDAlias is an alias for PropertyTenantID.
	PropertyTenantIDAlias = "tenantID"

	// PropertyCloud is the property name for the Azure cloud environment.
	PropertyCloud = "AZURE_CLOUD"
)
