// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"github.com/aws/aws-sdk-go-v2/service/route53"
	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var _ = Describe("region defaulting for workload identity", func() {
	It("should use the GovCloud region for a role in the aws-us-gov partition", func() {
		GinkgoT().Setenv("AWS_REGION", "")
		cfg := &provider.DNSHandlerConfig{
			Log: logr.Discard(),
			Properties: utils.Properties{
				securityv1alpha1constants.LabelWorkloadIdentityProvider: "aws",
				securityv1alpha1constants.DataKeyToken:                  "token",
			},
			Config: &runtime.RawExtension{Raw: []byte(`{}`)},
		}
		cfg.Properties[securityv1alpha1constants.DataKeyConfig] = "apiVersion: aws.provider.extensions.gardener.cloud/v1alpha1\n" +
			"kind: WorkloadIdentityConfig\n" +
			"roleARN: arn:aws-us-gov:iam::123456789012:role/dns\n"

		h, err := NewHandler(cfg)
		Expect(err).NotTo(HaveOccurred())
		client, ok := h.(*handler).r53.(*route53.Client)
		Expect(ok).To(BeTrue())
		Expect(client.Options().Region).To(Equal("us-gov-west-1"))
	})
})
