// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"encoding/base64"

	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var _ = Describe("Adapter", func() {
	var adapter provider.DNSHandlerAdapter

	BeforeEach(func() {
		adapter = newAdapter()
	})

	Describe("ValidateCredentialsAndProviderConfig", func() {
		Context("standard credentials", func() {
			It("should accept valid access key credentials", func() {
				props := utils.Properties{
					"AWS_ACCESS_KEY_ID": "AKIAIOSFODNN7EXAMPLE",
				}
				props["AWS_SECRET_ACCESS_KEY"] = base64.StdEncoding.EncodeToString([]byte("example-key"))

				Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(Succeed())
			})

			It("should accept valid region", func() {
				props := utils.Properties{
					"AWS_ACCESS_KEY_ID": "AKIAIOSFODNN7EXAMPLE",
					"AWS_REGION":        "us-east-1",
				}
				props["AWS_SECRET_ACCESS_KEY"] = base64.StdEncoding.EncodeToString([]byte("example-key"))

				Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(Succeed())
			})
		})

		Context("credentials chain", func() {
			It("should accept credentials chain", func() {
				props := utils.Properties{
					"AWS_USE_CREDENTIALS_CHAIN": "true",
				}

				Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(Succeed())
			})
		})

		Context("workload identity credentials", func() {
			It("should accept workload identity credentials", func() {
				props := utils.Properties{securityv1alpha1constants.DataKeyToken: "token-value",
					securityv1alpha1constants.DataKeyConfig:                 "config-value",
					securityv1alpha1constants.LabelWorkloadIdentityProvider: "aws",
				}

				Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(Succeed())
			})

			It("should NOT accept workload identity credentials without token", func() {
				props := utils.Properties{
					securityv1alpha1constants.DataKeyConfig:                 "config-value",
					securityv1alpha1constants.LabelWorkloadIdentityProvider: "aws",
				}

				Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(HaveOccurred())
			})

			It("should NOT accept workload identity credentials with wrong provider type", func() {
				props := utils.Properties{securityv1alpha1constants.DataKeyToken: "token-value",
					securityv1alpha1constants.DataKeyConfig:                 "config-value",
					securityv1alpha1constants.LabelWorkloadIdentityProvider: "gcp",
				}

				err := adapter.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("value must be \"aws\"")))
			})
		})
	})
})
