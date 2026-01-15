// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"

	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// code copied with minor modifications from https://github.com/gardener/gardener-extension-provider-gcp/blob/e72b6055c91b00cc4cb94ff14e1642075a12d4aa/pkg/apis/gcp/validation/workloadidentity.go

const (
	keyAudience                       = "audience"
	keyServiceAccountImpersonationURL = "service_account_impersonation_url"
	keySubjectTokenType               = "subject_token_type"
	keyTokenURL                       = "token_url"
	keyType                           = "type"
	keyUniverseDomain                 = "universe_domain"
)

var (
	workloadIdentityRequiredConfigFields = []string{ // Sorted alphabetically
		keyAudience,
		keySubjectTokenType,
		keyTokenURL,
		keyType,
		keyUniverseDomain,
	}
	workloadIdentityAllowedFields = append(workloadIdentityRequiredConfigFields, keyServiceAccountImpersonationURL)
)

// ValidateWorkloadIdentityConfig checks whether the given workload identity configuration contains expected fields and values.
func ValidateWorkloadIdentityConfig(config *WorkloadIdentityConfig, fldPath *field.Path, allowedTokenURLs []string, allowedServiceAccountImpersonationURLRegExps []*regexp.Regexp) field.ErrorList {
	allErrs := field.ErrorList{}

	if !projectIDRegexp.MatchString(config.ProjectID) {
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
		// clone the map and remove all allowed fields
		// if the cloned map has length greater than 0 then we have some extra fields in the original
		cloned := maps.Clone(cfg)
		for _, f := range workloadIdentityAllowedFields {
			delete(cloned, f)
		}
		if len(cloned) != 0 {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("credentialsConfig"), "contains extra fields, allowed fields are: "+strings.Join(workloadIdentityAllowedFields, ", ")))
		}

		// ensure that all required fields are present in the passed config
		for _, f := range workloadIdentityRequiredConfigFields {
			if _, ok := cfg[f]; !ok {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("credentialsConfig"), fmt.Sprintf("missing required field: %q", f)))
			}
		}

		if cfg[keyType] != externalAccountCredentialType {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("credentialsConfig").Child(keyType), cfg[keyType], fmt.Sprintf("should equal %q", externalAccountCredentialType)))
		}

		if cfg[keySubjectTokenType] != "urn:ietf:params:oauth:token-type:jwt" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("credentialsConfig").Child(keySubjectTokenType), cfg[keySubjectTokenType], fmt.Sprintf("should equal %q", "urn:ietf:params:oauth:token-type:jwt")))
		}

		retrievedTokenURL := cfg[keyTokenURL]
		rawTokenURL, ok := retrievedTokenURL.(string)
		if !ok {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("credentialsConfig").Child(keyTokenURL), cfg[keyTokenURL], "should be string"))
		}

		tokenURL, err := url.Parse(rawTokenURL)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("credentialsConfig").Child(keyTokenURL), cfg[keyTokenURL], "should be a valid URL"))
		}

		if tokenURL.Scheme != "https" {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("credentialsConfig").Child(keyTokenURL), cfg[keyTokenURL], "should start with https://"))
		}

		if !slices.Contains(allowedTokenURLs, rawTokenURL) {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("credentialsConfig").Child(keyTokenURL), fmt.Sprintf("allowed values are %q", allowedTokenURLs)))
		}

		if retrievedURL, ok := cfg[keyServiceAccountImpersonationURL]; ok {
			rawURL, ok := retrievedURL.(string)
			if !ok {
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("credentialsConfig").Child(keyServiceAccountImpersonationURL),
					cfg[keyServiceAccountImpersonationURL],
					"should be string"),
				)
			}

			allowed := slices.ContainsFunc(allowedServiceAccountImpersonationURLRegExps, func(allowedRegexp *regexp.Regexp) bool {
				return allowedRegexp.MatchString(rawURL)
			})

			if !allowed {
				regexpStrings := []string{}
				for _, r := range allowedServiceAccountImpersonationURLRegExps {
					regexpStrings = append(regexpStrings, r.String())
				}
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("credentialsConfig").Child(keyServiceAccountImpersonationURL),
					rawURL,
					"should match one of the allowed regular expressions: "+strings.Join(regexpStrings, ", ")),
				)
			}

			serviceAccountImpersonationURL, err := url.Parse(rawURL)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("credentialsConfig").Child(keyServiceAccountImpersonationURL),
					rawURL,
					"should be a valid URL"),
				)
			}

			if serviceAccountImpersonationURL.Scheme != "https" {
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("credentialsConfig").Child(keyServiceAccountImpersonationURL),
					rawURL,
					"should start with https://"),
				)
			}
		}
	}

	return allErrs
}

// ValidateWorkloadIdentityConfigUpdate validates updates on WorkloadIdentityConfig object.
func ValidateWorkloadIdentityConfigUpdate(oldConfig, newConfig *WorkloadIdentityConfig, fldPath *field.Path, allowedTokenURLs []string, allowedServiceAccountImpersonationURLRegExps []*regexp.Regexp) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, apivalidation.ValidateImmutableField(newConfig.ProjectID, oldConfig.ProjectID, fldPath.Child("projectID"))...)
	allErrs = append(allErrs, ValidateWorkloadIdentityConfig(newConfig, fldPath, allowedTokenURLs, allowedServiceAccountImpersonationURLRegExps)...)

	return allErrs
}
