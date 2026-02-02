// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package azure

import (
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
			It("should accept valid standard credentials", func() {
				props := utils.Properties{
					"AZURE_SUBSCRIPTION_ID": "12345678-1234-1234-1234-123456789012",
					"AZURE_CLIENT_ID":       "12345678-1234-1234-1234-123456789012",
					"AZURE_CLIENT_SECRET":   "secret",
					"AZURE_TENANT_ID":       "12345678-1234-1234-1234-123456789012",
				}

				Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(Succeed())
			})

			DescribeTable("should accept various Azure Cloud values", func(cloud string, ok bool) {
				props := utils.Properties{
					"AZURE_SUBSCRIPTION_ID": "12345678-1234-1234-1234-123456789012",
					"AZURE_CLIENT_ID":       "12345678-1234-1234-1234-123456789012",
					"AZURE_CLIENT_SECRET":   "secret",
					"AZURE_TENANT_ID":       "12345678-1234-1234-1234-123456789012",
					"AZURE_CLOUD":           cloud,
				}

				err := adapter.ValidateCredentialsAndProviderConfig(props, nil)
				if ok {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
				}
			},
				Entry("AzurePublic", "AzurePublic", true),
				Entry("AzureChina", "AzureChina", true),
				Entry("AzureGovernment", "AzureGovernment", true),
				Entry("Wrong", "foo", false),
			)
		})

		Context("workload identity credentials", func() {
			It("should accept workload identity credentials", func() {
				props := utils.Properties{securityv1alpha1constants.DataKeyToken: "token-value",
					securityv1alpha1constants.DataKeyConfig:                 "config-value",
					securityv1alpha1constants.LabelWorkloadIdentityProvider: "azure",
				}

				Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(Succeed())
			})

			It("should NOT accept workload identity credentials without token", func() {
				props := utils.Properties{
					securityv1alpha1constants.DataKeyConfig:                 "config-value",
					securityv1alpha1constants.LabelWorkloadIdentityProvider: "azure",
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
				Expect(err).To(MatchError(ContainSubstring("value must be \"azure\"")))
			})
		})
	})
})
