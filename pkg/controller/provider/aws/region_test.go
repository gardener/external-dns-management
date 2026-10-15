// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

var _ = Describe("region defaulting for workload identity", func() {
	newConfig := func(roleARN string) *provider.DNSHandlerConfig {
		return &provider.DNSHandlerConfig{
			Logger: logger.New(),
			Properties: utils.Properties{
				securityv1alpha1constants.LabelWorkloadIdentityProvider: "aws",
				securityv1alpha1constants.DataKeyConfig: "apiVersion: aws.provider.extensions.gardener.cloud/v1alpha1\n" +
					"kind: WorkloadIdentityConfig\n" +
					"roleARN: " + roleARN + "\n",
			},
		}
	}

	It("should use the GovCloud region for a role in the aws-us-gov partition", func() {
		Expect(defaultRegion(newConfig("arn:aws-us-gov:iam::123456789012:role/dns"))).To(Equal("us-gov-west-1"))
	})

	It("should use the China region for a role in the aws-cn partition", func() {
		Expect(defaultRegion(newConfig("arn:aws-cn:iam::123456789012:role/dns"))).To(Equal("cn-north-1"))
	})

	It("should use the commercial region for a role in the aws partition", func() {
		Expect(defaultRegion(newConfig("arn:aws:iam::123456789012:role/dns"))).To(Equal("us-west-2"))
	})

	It("should use the commercial region without workload identity", func() {
		Expect(defaultRegion(&provider.DNSHandlerConfig{})).To(Equal("us-west-2"))
	})
})
