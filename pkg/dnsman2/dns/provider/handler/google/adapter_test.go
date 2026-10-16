// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package google

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
		adapter = newAdapter(nil)
	})

	Describe("ValidateCredentialsAndProviderConfig", func() {
		Context("standard credentials", func() {
			It("should accept valid service account JSON", func() {
				serviceAccountJSON := `{
   "type": "service_account",
   "project_id": "my-project-123",
   "private_key_id": "key-id",
   "client_email": "test@my-project.iam.gserviceaccount.com"
  }`
				props := utils.Properties{
					"serviceaccount.json": serviceAccountJSON,
				}

				Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(Succeed())
			})

			It("should reject invalid JSON in service account", func() {
				props := utils.Properties{
					"serviceaccount.json": "invalid json",
				}

				err := adapter.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("does not contain a valid JSON")))
			})

			It("should reject service account with wrong type", func() {
				serviceAccountJSON := `{
   "type": "external_account",
   "project_id": "my-project-123",
   "private_key_id": "key-id",
   "client_email": "test@my-project.iam.gserviceaccount.com"
  }`
				props := utils.Properties{
					"serviceaccount.json": serviceAccountJSON,
				}

				err := adapter.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("not 'service_account'")))
			})
		})

		Context("token_uri", func() {
			saWithTokenURI := func(tokenURI string) utils.Properties {
				return utils.Properties{
					"serviceaccount.json": `{
   "type": "service_account",
   "project_id": "my-project-123",
   "private_key_id": "key-id",
   "client_email": "test@my-project.iam.gserviceaccount.com",
   "token_uri": "` + tokenURI + `"
  }`,
				}
			}

			Context("default allowlist", func() {
				It("should accept a Google token_uri", func() {
					props := saWithTokenURI("https://oauth2.googleapis.com/token")
					Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(Succeed())
				})

				It("should reject a non-Google token_uri", func() {
					props := saWithTokenURI("http://169.254.169.254/computeMetadata/v1/instance/service-accounts/default/token")
					err := adapter.ValidateCredentialsAndProviderConfig(props, nil)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(ContainSubstring("'token_uri' is not an allowed value")))
				})
			})

			Context("configured allowlist", func() {
				BeforeEach(func() {
					adapter = newAdapter([]string{"https://sts.example.com/token"})
				})

				It("should accept a token_uri on the configured allowlist", func() {
					props := saWithTokenURI("https://sts.example.com/token")
					Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(Succeed())
				})

				It("should reject a token_uri not on the configured allowlist, even a default one", func() {
					props := saWithTokenURI("https://oauth2.googleapis.com/token")
					err := adapter.ValidateCredentialsAndProviderConfig(props, nil)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(ContainSubstring("'token_uri' is not an allowed value")))
				})
			})
		})

		Context("workload identity credentials", func() {
			It("should accept workload identity credentials", func() {
				props := utils.Properties{securityv1alpha1constants.DataKeyToken: "token-value",
					securityv1alpha1constants.DataKeyConfig:                 "config-value",
					securityv1alpha1constants.LabelWorkloadIdentityProvider: "gcp",
				}

				Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(Succeed())
			})

			It("should NOT accept workload identity credentials without token", func() {
				props := utils.Properties{
					securityv1alpha1constants.DataKeyConfig:                 "config-value",
					securityv1alpha1constants.LabelWorkloadIdentityProvider: "gcp",
				}

				Expect(adapter.ValidateCredentialsAndProviderConfig(props, nil)).To(HaveOccurred())
			})

			It("should NOT accept workload identity credentials with wrong provider type", func() {
				props := utils.Properties{securityv1alpha1constants.DataKeyToken: "token-value",
					securityv1alpha1constants.DataKeyConfig:                 "config-value",
					securityv1alpha1constants.LabelWorkloadIdentityProvider: "aws",
				}

				err := adapter.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("value must be \"gcp\"")))
			})
		})
	})
})
