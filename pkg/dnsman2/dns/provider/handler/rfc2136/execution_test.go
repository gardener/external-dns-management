// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package rfc2136

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("sanitizeRFC2136Error", func() {
	It("returns only the generic message, never the net/DNS dial detail", func() {
		sanitized := sanitizeRFC2136Error("DNS exchange failed")
		Expect(sanitized.Error()).To(Equal("DNS exchange failed"))
		Expect(sanitized.Error()).NotTo(ContainSubstring("10.0.0.5"))
		Expect(sanitized.Error()).NotTo(ContainSubstring("dial"))
	})
})
