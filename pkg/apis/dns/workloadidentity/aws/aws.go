// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// WorkloadIdentityConfig contains configuration settings for AWS workload identity.
// copy from https://github.com/gardener/gardener-extension-provider-aws/blob/b2bfd4d78741a1bd14f7a1ac50cf3c4a89debd87/pkg/apis/aws/v1alpha1/types_workloadidentity.go
type WorkloadIdentityConfig struct {
	metav1.TypeMeta

	// RoleARN is the identifier of the role that the workload identity will assume.
	RoleARN string `json:"roleARN,omitempty"`
}

// DeepCopy creates a deep copy of the WorkloadIdentityConfig.
func (c *WorkloadIdentityConfig) DeepCopy() *WorkloadIdentityConfig {
	if c == nil {
		return nil
	}
	out := new(WorkloadIdentityConfig)
	out.TypeMeta = c.TypeMeta
	out.RoleARN = c.RoleARN
	return out
}

// ValidateWorkloadIdentityConfig checks whether the given aws workload identity configuration contains expected fields and values.
func ValidateWorkloadIdentityConfig(config *WorkloadIdentityConfig, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if config.APIVersion != "aws.provider.extensions.gardener.cloud/v1alpha1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), config.APIVersion, "apiVersion must be 'aws.provider.extensions.gardener.cloud/v1alpha1'"))
	}
	if config.Kind != "WorkloadIdentityConfig" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), config.Kind, "kind must be 'WorkloadIdentityConfig'"))
	}

	// code copied from https://github.com/gardener/gardener-extension-provider-aws/blob/42a1a953bad15c5daee3b591846069d8b1db08cc/pkg/apis/aws/validation/workloadidentity.go#L17-L19
	if len(config.RoleARN) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("roleARN"), "roleARN is required"))
	}

	return allErrs
}

// ValidateWorkloadIdentityConfigUpdate validates updates on WorkloadIdentityConfig object.
func ValidateWorkloadIdentityConfigUpdate(_, newConfig *WorkloadIdentityConfig, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, ValidateWorkloadIdentityConfig(newConfig, fldPath)...)

	return allErrs
}
