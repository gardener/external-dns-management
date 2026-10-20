// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package rfc2136

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRFC2136(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RFC2136 Handler Test Suite")
}
