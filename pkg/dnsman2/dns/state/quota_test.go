// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("QuotaReservationsMap", func() {
	var clock = testing.NewFakeClock(time.Now())

	var rmap *quotaReservationsMap
	const testTTL = 100 * time.Millisecond

	BeforeEach(func() {
		rmap = newQuotaReservationsMap(clock, testTTL)
	})

	Context("Reserve and Release operations", func() {
		It("should reserve entry for provider", func() {
			entryKey := client.ObjectKey{Namespace: "default", Name: "entry1"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			success := rmap.Reserve(entryKey, providerKey, func(count int32) bool {
				return count <= 10
			})
			Expect(success).To(BeTrue())

			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(int32(1)))
		})

		It("should release reservation", func() {
			entryKey := client.ObjectKey{Namespace: "default", Name: "entry1"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			rmap.Reserve(entryKey, providerKey, func(_ int32) bool { return true })
			rmap.Release(entryKey)

			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(int32(0)))
		})

		It("should reject reservation when allow function returns false", func() {
			entryKey := client.ObjectKey{Namespace: "default", Name: "entry1"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			success := rmap.Reserve(entryKey, providerKey, func(_ int32) bool {
				return false // Reject reservation
			})
			Expect(success).To(BeFalse())

			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(int32(0)))
		})
	})

	Context("CountReservationsForProvider", func() {
		It("should count reservations for specific provider", func() {
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entry2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			entry3 := client.ObjectKey{Namespace: "default", Name: "entry3"}
			provider1 := client.ObjectKey{Namespace: "default", Name: "provider1"}
			provider2 := client.ObjectKey{Namespace: "default", Name: "provider2"}

			rmap.Reserve(entry1, provider1, func(_ int32) bool { return true })
			rmap.Reserve(entry2, provider1, func(_ int32) bool { return true })
			rmap.Reserve(entry3, provider2, func(_ int32) bool { return true })

			count1 := rmap.CountReservationsForProvider(provider1)
			Expect(count1).To(Equal(int32(2)))

			count2 := rmap.CountReservationsForProvider(provider2)
			Expect(count2).To(Equal(int32(1)))
		})

		It("should return 0 for provider with no reservations", func() {
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}
			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(int32(0)))
		})
	})

	Context("TTL and expiration", func() {
		It("should expire reservations after TTL", func() {
			entryKey := client.ObjectKey{Namespace: "default", Name: "entry1"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			rmap.Reserve(entryKey, providerKey, func(_ int32) bool { return true })

			// Initially reservation exists
			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(int32(1)))

			// Wait for TTL to expire
			clock.Step(testTTL * 3 / 2)

			// Reservation should be cleaned up
			count = rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(int32(0)))
		})

		It("should clean up expired reservations during Reserve", func() {
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entry2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			// Create first reservation
			rmap.Reserve(entry1, providerKey, func(_ int32) bool { return true })
			Expect(rmap.CountReservationsForProvider(providerKey)).To(Equal(int32(1)))

			// Wait for expiration
			clock.Step(testTTL * 3 / 2)

			// Create second reservation - should trigger cleanup of first
			rmap.Reserve(entry2, providerKey, func(_ int32) bool { return true })

			// Should only have the new reservation
			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(int32(1)))
		})

		It("should allow reservation check function to see current count", func() {
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entry2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			// First reservation
			rmap.Reserve(entry1, providerKey, func(count int32) bool {
				Expect(count).To(Equal(int32(1))) // Including current reservation
				return count <= 2
			})

			// Second reservation should see count of 2
			rmap.Reserve(entry2, providerKey, func(count int32) bool {
				Expect(count).To(Equal(int32(2))) // Including current reservation
				return count <= 2
			})

			finalCount := rmap.CountReservationsForProvider(providerKey)
			Expect(finalCount).To(Equal(int32(2)))
		})

		It("should reject reservation when quota would be exceeded", func() {
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entry2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			entry3 := client.ObjectKey{Namespace: "default", Name: "entry3"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			quota := int32(2)

			// Reserve 2 entries (up to quota)
			rmap.Reserve(entry1, providerKey, func(count int32) bool { return count <= quota })
			rmap.Reserve(entry2, providerKey, func(count int32) bool { return count <= quota })

			// Third reservation should be rejected
			success := rmap.Reserve(entry3, providerKey, func(count int32) bool { return count <= quota })
			Expect(success).To(BeFalse())

			// Should still only have 2 reservations
			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(int32(2)))
		})
	})

	Context("Multiple providers", func() {
		It("should track reservations independently per provider", func() {
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entry2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			provider1 := client.ObjectKey{Namespace: "default", Name: "provider1"}
			provider2 := client.ObjectKey{Namespace: "default", Name: "provider2"}

			rmap.Reserve(entry1, provider1, func(_ int32) bool { return true })
			rmap.Reserve(entry2, provider2, func(_ int32) bool { return true })

			count1 := rmap.CountReservationsForProvider(provider1)
			count2 := rmap.CountReservationsForProvider(provider2)

			Expect(count1).To(Equal(int32(1)))
			Expect(count2).To(Equal(int32(1)))
		})
	})
})
