// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HostedZoneNameserversProvider", func() {
	Describe("NewHostedZoneNameserversProvider", func() {
		It("should get authoriative nameservers for given zone", func() {
			ctx := context.Background()
			provider, err := NewHostedZoneNameserversProvider(ctx, "example.com.", 5*time.Minute, SystemNameservers)
			Expect(err).NotTo(HaveOccurred())
			Expect(provider).NotTo(BeNil())

			nameservers, err := provider.Nameservers(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(nameservers).To(ConsistOf("elliott.ns.cloudflare.com.:53", "hera.ns.cloudflare.com.:53"))
		})
	})
})
