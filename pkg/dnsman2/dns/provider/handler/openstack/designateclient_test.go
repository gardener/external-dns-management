// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"errors"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("sanitizeOpenStackError", func() {
	It("strips the upstream response body but keeps the status code", func() {
		raw := gophercloud.ErrUnexpectedResponseCode{
			URL:    "http://169.254.169.254/latest/meta-data/",
			Method: "POST",
			Actual: 418,
			Body:   []byte(`{"secret":"metadata-oracle-leak"}`),
		}
		sanitized := sanitizeOpenStackError("authentication at Keystone failed", raw)

		Expect(sanitized.Error()).To(ContainSubstring("418"))
		Expect(sanitized.Error()).NotTo(ContainSubstring("metadata-oracle-leak"))
		Expect(sanitized.Error()).NotTo(ContainSubstring("169.254.169.254"))
	})

	It("drops arbitrary error text entirely", func() {
		sanitized := sanitizeOpenStackError("authentication at Keystone failed", errors.New("dial tcp 10.0.0.5:443: connect: connection refused"))

		Expect(sanitized.Error()).To(Equal("authentication at Keystone failed"))
		Expect(sanitized.Error()).NotTo(ContainSubstring("10.0.0.5"))
	})

	It("unwraps a wrapped unexpected-response-code error", func() {
		raw := gophercloud.ErrUnexpectedResponseCode{Actual: 500, Body: []byte("leak")}
		sanitized := sanitizeOpenStackError("authentication at Keystone failed", fmt.Errorf("wrapped: %w", raw))

		Expect(sanitized.Error()).To(ContainSubstring("500"))
		Expect(sanitized.Error()).NotTo(ContainSubstring("leak"))
	})
})
