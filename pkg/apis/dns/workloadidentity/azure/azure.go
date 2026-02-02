// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package azure

import (
	"fmt"
	"regexp"

	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"
)

var (
	guidRegex = regexp.MustCompile("^[0-9A-Fa-f]{8}-([0-9A-Fa-f]{4}-){3}[0-9A-Fa-f]{12}$")
)

// WorkloadIdentityConfig contains configuration settings for azure workload identity.
// copy from https://github.com/gardener/gardener-extension-provider-azure/blob/330df7a9af3f726ed00d9e3ddff5b945cbb01916/pkg/apis/azure/v1alpha1/types_workloadidentity.go
type WorkloadIdentityConfig struct {
	metav1.TypeMeta

	// ClientID is the ID of the Azure client.
	ClientID string `json:"clientID,omitempty"`
	// TenantID is the ID of the Azure tenant.
	TenantID string `json:"tenantID,omitempty"`
	// SubscriptionID is the ID of the subscription.
	SubscriptionID string `json:"subscriptionID,omitempty"`
}

func (c *WorkloadIdentityConfig) DeepCopy() *WorkloadIdentityConfig {
	if c == nil {
		return nil
	}
	out := new(WorkloadIdentityConfig)
	out.TypeMeta = c.TypeMeta
	out.ClientID = c.ClientID
	out.TenantID = c.TenantID
	out.SubscriptionID = c.SubscriptionID
	return out
}

// ValidateWorkloadIdentityConfig checks whether the given azure workload identity configuration contains expected fields and values.
func ValidateWorkloadIdentityConfig(config *WorkloadIdentityConfig, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if config.APIVersion != "azure.provider.extensions.gardener.cloud/v1alpha1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), config.APIVersion, "apiVersion must be 'azure.provider.extensions.gardener.cloud/v1alpha1'"))
	}
	if config.Kind != "WorkloadIdentityConfig" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), config.Kind, "kind must be 'WorkloadIdentityConfig'"))
	}

	// code copied from https://github.com/gardener/gardener-extension-provider-azure/blob/330df7a9af3f726ed00d9e3ddff5b945cbb01916/pkg/apis/azure/v1alpha1/types_workloadidentity.go
	if len(config.ClientID) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("clientID"), "clientID is required"))
	}
	if len(config.TenantID) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("tenantID"), "tenantID is required"))
	}
	if len(config.SubscriptionID) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("subscriptionID"), "subscriptionID is required"))
	}

	// clientID, tenantID and subscriptionID must be valid GUIDs,
	// see https://docs.microsoft.com/en-us/rest/api/securitycenter/locations/get
	if !guidRegex.Match([]byte(config.ClientID)) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("clientID"), config.ClientID, "clientID should be a valid GUID"))
	}
	if !guidRegex.Match([]byte(config.TenantID)) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("tenantID"), config.TenantID, "tenantID should be a valid GUID"))
	}
	if !guidRegex.Match([]byte(config.SubscriptionID)) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("subscriptionID"), config.SubscriptionID, "subscriptionID should be a valid GUID"))
	}

	return allErrs
}

// ValidateWorkloadIdentityConfigUpdate validates updates on WorkloadIdentityConfig object.
func ValidateWorkloadIdentityConfigUpdate(oldConfig, newConfig *WorkloadIdentityConfig, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newConfig.SubscriptionID, oldConfig.SubscriptionID, fldPath.Child("subscriptionID"))...)
	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newConfig.TenantID, oldConfig.TenantID, fldPath.Child("tenantID"))...)
	allErrs = append(allErrs, ValidateWorkloadIdentityConfig(newConfig, fldPath)...)

	return allErrs
}

// GetWorkloadIdentityConfig unmarshals and validates the given azure workload identity configuration data.
func GetWorkloadIdentityConfig(configData []byte) (*WorkloadIdentityConfig, error) {
	cfg := &WorkloadIdentityConfig{}
	if err := yaml.Unmarshal([]byte(configData), cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workload identity config: %w", err)
	}
	if err := ValidateWorkloadIdentityConfig(cfg, field.NewPath("config")).ToAggregate(); err != nil {
		return nil, err
	}
	return cfg, nil
}
