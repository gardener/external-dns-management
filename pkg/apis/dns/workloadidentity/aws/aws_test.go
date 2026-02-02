// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws_test

import (
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	. "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/aws"
)

var _ = Describe("#ValidateWorkloadIdentityConfig", func() {
	var (
		workloadIdentityConfig *WorkloadIdentityConfig
	)

	BeforeEach(func() {
		workloadIdentityConfig = &WorkloadIdentityConfig{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "aws.provider.extensions.gardener.cloud/v1alpha1",
				Kind:       "WorkloadIdentityConfig",
			},
			RoleARN: "foo",
		}
	})

	It("should validate the config successfully", func() {
		Expect(ValidateWorkloadIdentityConfig(workloadIdentityConfig, field.NewPath(""))).To(BeEmpty())
	})

	It("should contain all expected validation errors", func() {
		workloadIdentityConfig.Kind = ""
		workloadIdentityConfig.APIVersion = ""
		workloadIdentityConfig.RoleARN = ""
		errorList := ValidateWorkloadIdentityConfig(workloadIdentityConfig, field.NewPath("providerConfig"))
		Expect(errorList).To(ConsistOfFields(
			Fields{
				"Type":   Equal(field.ErrorTypeRequired),
				"Field":  Equal("providerConfig.roleARN"),
				"Detail": Equal("roleARN is required"),
			},
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("providerConfig.apiVersion"),
				"BadValue": Equal(""),
				"Detail":   Equal("apiVersion must be 'aws.provider.extensions.gardener.cloud/v1alpha1'"),
			},
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("providerConfig.kind"),
				"BadValue": Equal(""),
				"Detail":   Equal("kind must be 'WorkloadIdentityConfig'"),
			},
		))
	})

	It("should validate the config successfully during update", func() {
		newConfig := workloadIdentityConfig.DeepCopy()
		Expect(ValidateWorkloadIdentityConfigUpdate(workloadIdentityConfig, newConfig, field.NewPath(""))).To(BeEmpty())
	})

	It("should allow changing the roleARN during update", func() {
		newConfig := workloadIdentityConfig.DeepCopy()
		newConfig.RoleARN = "bar"
		errorList := ValidateWorkloadIdentityConfigUpdate(workloadIdentityConfig, newConfig, field.NewPath("providerConfig"))
		Expect(errorList).To(BeEmpty())
	})
})
