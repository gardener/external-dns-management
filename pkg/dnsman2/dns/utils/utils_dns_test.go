// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var _ = Describe("UniqueStrings", func() {
	Describe("#Difference", func() {
		var (
			u1, u2 *utils.UniqueStrings
		)

		BeforeEach(func() {
			u1 = utils.NewUniqueStrings()
			u2 = utils.NewUniqueStrings()
		})

		It("should handle empty sets", func() {
			diff := u1.Difference(u2)
			Expect(diff).To(BeEmpty())
		})

		It("should compute the difference correctly", func() {
			u1.AddAll([]string{"a", "b", "c"})
			u2.AddAll([]string{"b", "c", "d"})
			diff := u1.Difference(u2)
			Expect(diff).To(ConsistOf("a"))
		})

		It("should handle different sets", func() {
			u1.AddAll([]string{"a", "b", "c"})
			u2.AddAll([]string{"x", "y", "z"})
			diff := u1.Difference(u2)
			Expect(diff).To(ConsistOf("a", "b", "c"))
		})

		It("should handle identical sets", func() {
			u1.AddAll([]string{"a", "b", "c"})
			u2.AddAll([]string{"a", "b", "c"})
			diff := u1.Difference(u2)
			Expect(diff).To(BeEmpty())
		})
	})
})
