// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package providerselector

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProviderSelector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DNSEntry ProviderSelector Suite")
}
