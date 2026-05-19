// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
)

func TestApp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "App Suite")
}

var _ = Describe("Options Validation", func() {
	Describe("#Validate", func() {
		DescribeTable("validation scenarios",
			func(primaryClass string, secondaryClasses []string, shouldSucceed bool, errorSubstring string) {
				opts := &options{
					config: &config.DNSManagerConfiguration{
						Class:            primaryClass,
						SecondaryClasses: secondaryClasses,
					},
				}

				err := opts.Validate()
				if shouldSucceed {
					Expect(err).To(Succeed())
				} else {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(errorSubstring))
				}
			},
			Entry("no secondary classes", "primary", nil, true, ""),
			Entry("valid secondary classes", "primary", []string{"secondary1", "secondary2"}, true, ""),
			Entry("multiple different secondary classes", "primary", []string{"secondary1", "secondary2", "secondary3"}, true, ""),
			Entry("secondary class equals primary class", "primary", []string{"primary"}, false, "equivalent to primary class"),
			Entry("secondary class equivalent to primary (default class)", "", []string{"gardendns"}, false, "equivalent to primary class"),
			Entry("secondary class equivalent to default class", "gardendns", []string{""}, false, "equivalent to primary class"),
			Entry("duplicate secondary classes (exact match)", "primary", []string{"secondary1", "secondary1"}, false, "duplicate secondary class found"),
			Entry("duplicate secondary classes (empty strings)", "primary", []string{"", ""}, false, "duplicate secondary class found"),
			Entry("equivalent secondary classes (default variations)", "primary", []string{"gardendns", ""}, false, "duplicate secondary class found"),
		)
	})
})
