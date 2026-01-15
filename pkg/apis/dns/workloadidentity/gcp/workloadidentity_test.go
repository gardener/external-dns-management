// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gcp_test

import (
	"regexp"

	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/gcp"
)

var _ = Describe("#ValidateWorkloadIdentityConfig", func() {
	var (
		workloadIdentityConfig                       *gcp.WorkloadIdentityConfig
		allowedTokenURLs                             = []string{"https://sts.googleapis.com/v1/token"}
		allowedServiceAccountImpersonationURLRegExps = []*regexp.Regexp{regexp.MustCompile(`^https://iamcredentials\.googleapis\.com/v1/projects/-/serviceAccounts/.+:generateAccessToken$`)}
	)

	BeforeEach(func() {
		workloadIdentityConfig = &gcp.WorkloadIdentityConfig{
			ProjectID: "my-project",
			CredentialsConfig: &runtime.RawExtension{
				Raw: []byte(`
{
	"universe_domain": "googleapis.com",
	"type": "external_account",
	"audience": "//iam.googleapis.com/projects/11111111/locations/global/workloadIdentityPools/foopool/providers/fooprovider",
	"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
	"token_url": "https://sts.googleapis.com/v1/token"
}
`),
			},
		}
	})

	It("should validate the config successfully", func() {
		Expect(gcp.ValidateWorkloadIdentityConfig(workloadIdentityConfig, field.NewPath(""), allowedTokenURLs, allowedServiceAccountImpersonationURLRegExps)).To(BeEmpty())
	})

	It("should contain all expected validation errors", func() {
		workloadIdentityConfig.ProjectID = "_invalid"
		workloadIdentityConfig.CredentialsConfig.Raw = []byte(`
{
	"type": "not_external_account",
	"audience": "//iam.googleapis.com/projects/11111111/locations/global/workloadIdentityPools/foopool/providers/fooprovider",
	"subject_token_type": "invalid",
	"token_url": "http://insecure",
	"service_account_impersonation_url": "http://insecure",
	"credential_source": {
		"file": "/abc/cloudprovider/xyz",
		"abc": {
			"foo": "text"
		}
	}
}
`)
		errorList := gcp.ValidateWorkloadIdentityConfig(workloadIdentityConfig, field.NewPath("providerConfig"), []string{"https://foo.bar.real.api/token"}, []*regexp.Regexp{regexp.MustCompile("https://does-not-match")})
		Expect(errorList).To(ConsistOfFields(
			Fields{
				"Type":   Equal(field.ErrorTypeForbidden),
				"Field":  Equal("providerConfig.credentialsConfig"),
				"Detail": Equal("missing required field: \"universe_domain\""),
			},
			Fields{
				"Type":   Equal(field.ErrorTypeInvalid),
				"Field":  Equal("providerConfig.credentialsConfig.type"),
				"Detail": Equal("should equal \"external_account\""),
			},
			Fields{
				"Type":   Equal(field.ErrorTypeForbidden),
				"Field":  Equal("providerConfig.credentialsConfig"),
				"Detail": Equal("contains extra fields, allowed fields are: audience, subject_token_type, token_url, type, universe_domain, service_account_impersonation_url"),
			},
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("providerConfig.projectID"),
				"BadValue": Equal("_invalid"),
				"Detail":   Equal("does not match the expected format"),
			},
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("providerConfig.credentialsConfig.subject_token_type"),
				"BadValue": Equal("invalid"),
				"Detail":   Equal("should equal \"urn:ietf:params:oauth:token-type:jwt\""),
			},
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("providerConfig.credentialsConfig.token_url"),
				"BadValue": Equal("http://insecure"),
				"Detail":   Equal("should start with https://"),
			},
			Fields{
				"Type":   Equal(field.ErrorTypeForbidden),
				"Field":  Equal("providerConfig.credentialsConfig.token_url"),
				"Detail": Equal("allowed values are [\"https://foo.bar.real.api/token\"]"),
			},
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("providerConfig.credentialsConfig.service_account_impersonation_url"),
				"BadValue": Equal("http://insecure"),
				"Detail":   Equal("should start with https://"),
			},
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("providerConfig.credentialsConfig.service_account_impersonation_url"),
				"BadValue": Equal("http://insecure"),
				"Detail":   Equal("should match one of the allowed regular expressions: https://does-not-match"),
			},
		))
	})

	It("should return an validation error if CredentialsConfig is not set", func() {
		workloadIdentityConfig.ProjectID = "my-project"
		workloadIdentityConfig.CredentialsConfig = nil

		errorList := gcp.ValidateWorkloadIdentityConfig(workloadIdentityConfig, field.NewPath("providerConfig"), nil, nil)
		Expect(errorList).To(ConsistOfFields(
			Fields{
				"Type":     Equal(field.ErrorTypeRequired),
				"Field":    Equal("providerConfig.credentialsConfig"),
				"BadValue": Equal(""),
				"Detail":   Equal("is required"),
			},
		))
	})

	It("should validate the config successfully during update", func() {
		newConfig := workloadIdentityConfig.DeepCopy()
		Expect(gcp.ValidateWorkloadIdentityConfigUpdate(workloadIdentityConfig, newConfig, field.NewPath(""), allowedTokenURLs, allowedServiceAccountImpersonationURLRegExps)).To(BeEmpty())
	})

	It("should not allow chaning the projectID during update", func() {
		newConfig := workloadIdentityConfig.DeepCopy()
		newConfig.ProjectID = "valid123"
		errorList := gcp.ValidateWorkloadIdentityConfigUpdate(workloadIdentityConfig, newConfig, field.NewPath("providerConfig"), allowedTokenURLs, allowedServiceAccountImpersonationURLRegExps)

		Expect(errorList).To(ConsistOfFields(
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("providerConfig.projectID"),
				"BadValue": Equal("valid123"),
				"Detail":   Equal("field is immutable"),
			},
		))
	})
})
