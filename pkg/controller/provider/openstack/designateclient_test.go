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
