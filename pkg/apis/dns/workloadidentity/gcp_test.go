// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package workloadidentity_test

import (
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	. "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity"
)

var _ = Describe("#ValidateWorkloadIdentityConfig", func() {
	var (
		workloadIdentityConfig *GCPWorkloadIdentityConfig
	)

	BeforeEach(func() {
		workloadIdentityConfig = &GCPWorkloadIdentityConfig{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "gcp.provider.extensions.gardener.cloud/v1alpha1",
				Kind:       "WorkloadIdentityConfig",
			},
			ProjectID: "my-project",
			CredentialsConfig: &runtime.RawExtension{
				Raw: []byte(`
{
	"universe_domain": "googleapis.com",
	"type": "external_account",
	"audience": "//iam.googleapis.com/projects/11111111/locations/global/workloadIdentityPools/foopool/providers/fooprovider",
	"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
	"token_url": "https://sts.googleapis.com/v1/token",
	"credential_source": {
		"file": "/abc/cloudprovider/xyz",
		"abc": {
		  "foo": "text"
		}
	}
}
`),
			},
		}
	})

	It("should validate the config successfully", func() {
		Expect(ValidateGCPWorkloadIdentityConfig(workloadIdentityConfig, field.NewPath(""))).To(BeEmpty())
	})

	It("should contain all expected validation errors", func() {
		workloadIdentityConfig.Kind = ""
		workloadIdentityConfig.APIVersion = ""
		workloadIdentityConfig.ProjectID = "_invalid"
		workloadIdentityConfig.CredentialsConfig.Raw = []byte(`
{
	"extra": "field",
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
		errorList := ValidateGCPWorkloadIdentityConfig(workloadIdentityConfig, field.NewPath("providerConfig"))
		Expect(errorList).To(ConsistOfFields(
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("providerConfig.apiVersion"),
				"BadValue": Equal(""),
				"Detail":   Equal("apiVersion must be 'gcp.provider.extensions.gardener.cloud/v1alpha1'"),
			},
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("providerConfig.kind"),
				"BadValue": Equal(""),
				"Detail":   Equal("kind must be 'WorkloadIdentityConfig'"),
			},
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
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("providerConfig.credentialsConfig.service_account_impersonation_url"),
				"BadValue": Equal("http://insecure"),
				"Detail":   Equal("should start with https://"),
			},
		))
	})

	It("should validate the config successfully during update", func() {
		newConfig := workloadIdentityConfig.DeepCopy()
		Expect(ValidateGCPWorkloadIdentityConfigUpdate(workloadIdentityConfig, newConfig, field.NewPath(""))).To(BeEmpty())
	})

	It("should not allow changing the projectID during update", func() {
		newConfig := workloadIdentityConfig.DeepCopy()
		newConfig.ProjectID = "valid123"
		errorList := ValidateGCPWorkloadIdentityConfigUpdate(workloadIdentityConfig, newConfig, field.NewPath("providerConfig"))

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
