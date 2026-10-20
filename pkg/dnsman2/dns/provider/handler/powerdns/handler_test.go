// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package powerdns

import (
	"errors"
	"fmt"

	"github.com/joeig/go-powerdns/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("sanitizePowerDNSError", func() {
	const msg = "listing zones at PowerDNS failed"

	It("keeps the HTTP status code and drops the upstream message body", func() {
		httpErr := &powerdns.Error{StatusCode: 418, Status: "418 I'm a teapot", Message: "metadata-oracle-leak"}
		sanitized := sanitizePowerDNSError(msg, httpErr)
		Expect(sanitized.Error()).To(ContainSubstring("418"))
		Expect(sanitized.Error()).NotTo(ContainSubstring("metadata-oracle-leak"))
	})

	It("unwraps a wrapped typed error before sanitizing", func() {
		httpErr := &powerdns.Error{StatusCode: 418, Message: "metadata-oracle-leak"}
		sanitized := sanitizePowerDNSError(msg, fmt.Errorf("wrapped: %w", httpErr))
		Expect(sanitized.Error()).To(ContainSubstring("418"))
		Expect(sanitized.Error()).NotTo(ContainSubstring("metadata-oracle-leak"))
	})

	It("drops connection-level error text entirely", func() {
		sanitized := sanitizePowerDNSError(msg, errors.New("dial tcp 10.0.0.5:443: connect: connection refused"))
		Expect(sanitized.Error()).To(Equal(msg))
		Expect(sanitized.Error()).NotTo(ContainSubstring("10.0.0.5"))
	})
})
