// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package infoblox

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInfoblox(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Infoblox Handler Test Suite")
}
