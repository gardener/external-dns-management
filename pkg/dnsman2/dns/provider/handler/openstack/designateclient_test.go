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

var _ = Describe("denyNonGlobalUnicastDial", func() {
	DescribeTable("denies non-public destinations",
		func(addr string) {
			Expect(denyNonGlobalUnicastDial("tcp", addr, nil)).To(HaveOccurred())
		},
		Entry("cloud metadata (link-local)", "169.254.169.254:80"),
		Entry("loopback", "127.0.0.1:443"),
		Entry("IPv6 loopback", "[::1]:443"),
		Entry("unspecified", "0.0.0.0:443"),
		Entry("multicast", "224.0.0.1:443"),
	)

	DescribeTable("allows public destinations",
		func(addr string) {
			Expect(denyNonGlobalUnicastDial("tcp", addr, nil)).NotTo(HaveOccurred())
		},
		Entry("public IPv4", "93.184.216.34:443"),
		Entry("public IPv6", "[2606:2800:220:1::]:443"),
		Entry("private 10/8", "10.0.0.5:443"),
		Entry("private 192.168/16", "192.168.1.1:443"),
		Entry("private 172.16/12", "172.16.0.1:443"),
		Entry("IPv6 unique-local", "[fd00::1]:443"),
	)
})
