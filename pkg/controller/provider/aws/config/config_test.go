// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/controller/provider/aws/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AWS Config Suite")
}

var _ = Describe("AllowCredentialsChain", func() {
	It("is disabled and errorless when unset", func() {
		GinkgoT().Setenv(config.EnvAllowCredentialsChain, "")
		enabled, err := config.AllowCredentialsChain()
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeFalse())
	})

	It("is enabled for a truthy value (whitespace tolerated)", func() {
		GinkgoT().Setenv(config.EnvAllowCredentialsChain, " true ")
		enabled, err := config.AllowCredentialsChain()
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())
	})

	It("errors (fail closed) for an unparseable value", func() {
		GinkgoT().Setenv(config.EnvAllowCredentialsChain, "yes")
		enabled, err := config.AllowCredentialsChain()
		Expect(err).To(HaveOccurred())
		Expect(enabled).To(BeFalse())
	})
})
