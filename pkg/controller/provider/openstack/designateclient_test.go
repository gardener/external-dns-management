// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	. "github.com/onsi/gomega"
)

func TestSanitizeOpenStackError(t *testing.T) {
	RegisterTestingT(t)

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

	// wrapped unexpected-response-code error is still unwrapped and sanitized
	wrapped := sanitizeOpenStackError("authentication at Keystone failed", fmt.Errorf("wrapped: %w", raw))
	Expect(wrapped.Error()).To(ContainSubstring("418"))
	Expect(wrapped.Error()).NotTo(ContainSubstring("metadata-oracle-leak"))

	// arbitrary error text is dropped entirely
	other := sanitizeOpenStackError("authentication at Keystone failed", errors.New("dial tcp 10.0.0.5:443: connect: connection refused"))
	Expect(other.Error()).To(Equal("authentication at Keystone failed"))
	Expect(other.Error()).NotTo(ContainSubstring("10.0.0.5"))
}

func TestDenyNonGlobalUnicastDial(t *testing.T) {
	RegisterTestingT(t)

	denied := []string{
		"169.254.169.254:80", // cloud metadata (link-local)
		"127.0.0.1:443",      // loopback
		"[::1]:443",          // IPv6 loopback
		"0.0.0.0:443",        // unspecified
		"224.0.0.1:443",      // multicast
	}
	for _, addr := range denied {
		Expect(denyNonGlobalUnicastDial("tcp", addr, nil)).To(HaveOccurred(), "expected %s to be denied", addr)
	}

	allowed := []string{
		"93.184.216.34:443",       // public IPv4
		"[2606:2800:220:1::]:443", // public IPv6
		"10.0.0.5:443",            // private
		"192.168.1.1:443",         // private
		"172.16.0.1:443",          // private
		"[fd00::1]:443",            // IPv6 unique-local
	}
	for _, addr := range allowed {
		Expect(denyNonGlobalUnicastDial("tcp", addr, nil)).NotTo(HaveOccurred(), "expected %s to be allowed", addr)
	}
}
