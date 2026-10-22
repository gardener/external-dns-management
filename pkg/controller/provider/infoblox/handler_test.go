// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package infoblox

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("sanitizeInfobloxError", func() {
	It("returns only the generic message, never the untyped WAPI response body", func() {
		sanitized := sanitizeInfobloxError("listing zones at Infoblox failed")
		Expect(sanitized.Error()).To(Equal("listing zones at Infoblox failed"))
		Expect(sanitized.Error()).NotTo(ContainSubstring("Contents"))
		Expect(sanitized.Error()).NotTo(ContainSubstring("WAPI request error"))
	})
})
