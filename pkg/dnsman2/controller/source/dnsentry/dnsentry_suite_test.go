// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLandscape(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DNSEntry Source Controller Suite")
}
