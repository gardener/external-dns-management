// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package data_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/aws/data"
)

var _ = Describe("CanonicalHostedZones", func() {
	It("has disjunct keys between enabled and disabled zones", func() {
		enabled := data.CanonicalHostedZones()
		for suffix := range data.DisabledCanonicalHostedZones() {
			Expect(enabled).NotTo(HaveKey(suffix), "suffix %q must not be both enabled and disabled", suffix)
		}
	})
})
