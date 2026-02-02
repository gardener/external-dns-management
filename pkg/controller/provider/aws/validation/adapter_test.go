// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"encoding/base64"
	"encoding/json"

	"github.com/gardener/controller-manager-library/pkg/utils"
	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/controller/provider/aws/config"
	. "github.com/gardener/external-dns-management/pkg/controller/provider/aws/validation"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

var _ = Describe("Adapter", func() {
	var adapter provider.DNSHandlerAdapter

	BeforeEach(func() {
		adapter = NewAdapter()
	})

	Describe("ValidateCredentialsAndProviderConfig", func() {
		Context("with valid provider config", func() {
			It("should accept valid batch size", func() {
				cfg := config.AWSConfig{BatchSize: 25}
				raw, err := json.Marshal(cfg)
				Expect(err).NotTo(HaveOccurred())

				Expect(adapter.ValidateCredentialsAndProviderConfig(nil, &runtime.RawExtension{Raw: raw})).To(Succeed())
			})

			It("should accept minimum batch size of 1", func() {
				cfg := config.AWSConfig{BatchSize: 1}
				raw, err := json.Marshal(cfg)
				Expect(err).NotTo(HaveOccurred())

				Expect(adapter.ValidateCredentialsAndProviderConfig(nil, &runtime.RawExtension{Raw: raw})).To(Succeed())
			})

			It("should accept maximum batch size of 50", func() {
				cfg := config.AWSConfig{BatchSize: 50}
				raw, err := json.Marshal(cfg)
				Expect(err).NotTo(HaveOccurred())

				Expect(adapter.ValidateCredentialsAndProviderConfig(nil, &runtime.RawExtension{Raw: raw})).To(Succeed())
			})
		})

		Context("with invalid provider config", func() {
			It("should reject batch size of 0", func() {
				cfg := config.AWSConfig{BatchSize: 0}
				raw, err := json.Marshal(cfg)
				Expect(err).NotTo(HaveOccurred())

				err = adapter.ValidateCredentialsAndProviderConfig(nil, &runtime.RawExtension{Raw: raw})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("invalid batch size")))
			})

			It("should reject batch size greater than 50", func() {
				cfg := config.AWSConfig{BatchSize: 51}
				raw, err := json.Marshal(cfg)
				Expect(err).NotTo(HaveOccurred())

				err = adapter.ValidateCredentialsAndProviderConfig(nil, &runtime.RawExtension{Raw: raw})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("invalid batch size")))
			})

			It("should reject invalid JSON", func() {
				err := adapter.ValidateCredentialsAndProviderConfig(nil, &runtime.RawExtension{Raw: []byte("invalid json")})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("unmarshal providerConfig failed")))
			})
		})

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
