// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package workloadidentity

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"strings"

	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	gcpKeyAudience                       = "audience"
	gcpKeyServiceAccountImpersonationURL = "service_account_impersonation_url"
	gcpKeySubjectTokenType               = "subject_token_type"
	gcpKeyTokenURL                       = "token_url"
	gcpKeyType                           = "type"
	gcpKeyUniverseDomain                 = "universe_domain"

	gcpExternalAccountCredentialType = "external_account"
)

var (
	gcpRequiredConfigFields = []string{ // Sorted alphabetically
		gcpKeyAudience,
		gcpKeySubjectTokenType,
		gcpKeyTokenURL,
		gcpKeyType,
		gcpKeyUniverseDomain,
	}
	gcpAllowedFields = append(gcpRequiredConfigFields, gcpKeyServiceAccountImpersonationURL)
)

var gcpProjectIDRegexp = regexp.MustCompile(`^(?P<project>[a-z][a-z0-9-]{4,28}[a-z0-9])$`)

// GCPWorkloadIdentityConfig contains configuration settings for GCP workload identity.
// copy from https://github.com/gardener/gardener-extension-provider-gcp/blob/6e2574698ed9e3b75734dcc5eaa7dd93c39f00fd/pkg/apis/gcp/v1alpha1/types_workloadidentity.go
type GCPWorkloadIdentityConfig struct {
	metav1.TypeMeta `json:",inline"`

	// ProjectID is the ID of the GCP project.
	ProjectID string `json:"projectID,omitempty"`
	// CredentialsConfig contains information for workload authentication against GCP.
	CredentialsConfig *runtime.RawExtension `json:"credentialsConfig,omitempty"`
}

// DeepCopy creates a deep copy of the GCPWorkloadIdentityConfig.
func (c *GCPWorkloadIdentityConfig) DeepCopy() *GCPWorkloadIdentityConfig {
	if c == nil {
		return nil
	}
	out := new(GCPWorkloadIdentityConfig)
	out.TypeMeta = c.TypeMeta
	out.ProjectID = c.ProjectID
	if c.CredentialsConfig != nil {
		out.CredentialsConfig = c.CredentialsConfig.DeepCopy()
	}
	return out
}

// ValidateGCPWorkloadIdentityConfig checks whether the given GCP workload identity configuration contains expected fields and values.
func ValidateGCPWorkloadIdentityConfig(config *GCPWorkloadIdentityConfig, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if config.APIVersion != "gcp.provider.extensions.gardener.cloud/v1alpha1" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), config.APIVersion, "apiVersion must be 'gcp.provider.extensions.gardener.cloud/v1alpha1'"))
	}
	if config.Kind != "WorkloadIdentityConfig" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("kind"), config.Kind, "kind must be 'WorkloadIdentityConfig'"))
	}

	// code copied from https://github.com/gardener/gardener-extension-provider-gcp/blob/58c50a8c8511d97cbf2bc7d97ee1b8df8737e3c8/pkg/apis/gcp/validation/workloadidentity.go#L47-L147
	if !gcpProjectIDRegexp.MatchString(config.ProjectID) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("projectID"), config.ProjectID, "does not match the expected format"))
	}

	if config.CredentialsConfig == nil {
		allErrs = append(allErrs, field.Required(fldPath.Child("credentialsConfig"), "is required"))
		return allErrs
	}

	cfg := map[string]any{}
	if err := json.Unmarshal(config.CredentialsConfig.Raw, &cfg); err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("credentialsConfig"), config.CredentialsConfig.Raw, "has invalid format"))
	} else {
		// we do not care about this field since it will be overwritten anyways
		delete(cfg, "credential_source")

		cloned := maps.Clone(cfg)
		for _, f := range gcpAllowedFields {
			delete(cloned, f)
		}
		if len(cloned) != 0 {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("credentialsConfig"), "contains extra fields, allowed fields are: "+strings.Join(gcpAllowedFields, ", ")))
		}

		for _, f := range gcpRequiredConfigFields {
			if _, ok := cfg[f]; !ok {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("credentialsConfig"), fmt.Sprintf("missing required field: %q", f)))
			}
		}

		if cfg[gcpKeyType] != gcpExternalAccountCredentialType {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("credentialsConfig").Child(gcpKeyType), cfg[gcpKeyType], fmt.Sprintf("should equal %q", gcpExternalAccountCredentialType)))
		}

		if cfg[gcpKeySubjectTokenType] != "urn:ietf:params:oauth:token-type:jwt" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("credentialsConfig").Child(gcpKeySubjectTokenType), cfg[gcpKeySubjectTokenType], fmt.Sprintf("should equal %q", "urn:ietf:params:oauth:token-type:jwt")))
		}

		retrievedTokenURL := cfg[gcpKeyTokenURL]
		rawTokenURL, ok := retrievedTokenURL.(string)
		if !ok {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("credentialsConfig").Child(gcpKeyTokenURL), cfg[gcpKeyTokenURL], "should be string"))
		}
		tokenURL, err := url.Parse(rawTokenURL)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("credentialsConfig").Child(gcpKeyTokenURL), cfg[gcpKeyTokenURL], "should be a valid URL"))
		}

		if tokenURL.Scheme != "https" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("credentialsConfig").Child(gcpKeyTokenURL), cfg[gcpKeyTokenURL], "should start with https://"))
		}

		if retrievedURL, ok := cfg[gcpKeyServiceAccountImpersonationURL]; ok {
			rawURL, ok := retrievedURL.(string)
			if !ok {
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("credentialsConfig").Child(gcpKeyServiceAccountImpersonationURL),
					cfg[gcpKeyServiceAccountImpersonationURL],
					"should be string"),
				)
			}
			serviceAccountImpersonationURL, err := url.Parse(rawURL)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("credentialsConfig").Child(gcpKeyServiceAccountImpersonationURL),
					cfg[gcpKeyServiceAccountImpersonationURL],
					"should be a valid URL"),
				)
			}

			if serviceAccountImpersonationURL.Scheme != "https" {
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("credentialsConfig").Child(gcpKeyServiceAccountImpersonationURL),
					cfg[gcpKeyServiceAccountImpersonationURL],
					"should start with https://"),
				)
			}
		}
	}

	return allErrs
}

// ValidateGCPWorkloadIdentityConfigUpdate validates updates on GCP WorkloadIdentityConfig object.
func ValidateGCPWorkloadIdentityConfigUpdate(oldConfig, newConfig *GCPWorkloadIdentityConfig, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newConfig.ProjectID, oldConfig.ProjectID, fldPath.Child("projectID"))...)
	allErrs = append(allErrs, ValidateGCPWorkloadIdentityConfig(newConfig, fldPath)...)

	return allErrs
}
