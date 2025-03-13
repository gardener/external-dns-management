// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HostedZoneNameserversProvider", func() {
	Describe("NewHostedZoneNameserversProvider", func() {
		It("should get authoriative nameservers for given zone", func() {
			provider, err := NewHostedZoneNameserversProvider("example.com.", 5*time.Minute, SystemNameservers)
			Expect(err).NotTo(HaveOccurred())
			Expect(provider).NotTo(BeNil())

			nameservers, err := provider.Nameservers()
			Expect(err).NotTo(HaveOccurred())
			Expect(nameservers).To(ConsistOf("a.iana-servers.net.", "b.iana-servers.net."))
		})
	})
})
