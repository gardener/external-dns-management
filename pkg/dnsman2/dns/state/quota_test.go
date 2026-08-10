// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"
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

	allowUpTo := func(quota int) func(sets.Set[client.ObjectKey]) bool {
		return func(keys sets.Set[client.ObjectKey]) bool {
			return keys.Len() <= quota
		}
	}
	allowAll := allowUpTo(1000)

	Context("Reserve and Release operations", func() {
		It("should reserve entry for provider", func() {
			entryKey := client.ObjectKey{Namespace: "default", Name: "entry1"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			success := rmap.Reserve(entryKey, providerKey, allowUpTo(10))
			Expect(success).To(BeTrue())

			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(1))
		})

		It("should release reservation", func() {
			entryKey := client.ObjectKey{Namespace: "default", Name: "entry1"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			rmap.Reserve(entryKey, providerKey, allowAll)
			rmap.Release(entryKey)

			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(0))
		})

		It("should reject reservation when allow function returns false", func() {
			entryKey := client.ObjectKey{Namespace: "default", Name: "entry1"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			success := rmap.Reserve(entryKey, providerKey, func(_ sets.Set[client.ObjectKey]) bool {
				return false
			})
			Expect(success).To(BeFalse())

			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(0))
		})
	})

	Context("CountReservationsForProvider", func() {
		It("should count reservations for specific provider", func() {
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entry2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			entry3 := client.ObjectKey{Namespace: "default", Name: "entry3"}
			provider1 := client.ObjectKey{Namespace: "default", Name: "provider1"}
			provider2 := client.ObjectKey{Namespace: "default", Name: "provider2"}

			rmap.Reserve(entry1, provider1, allowAll)
			rmap.Reserve(entry2, provider1, allowAll)
			rmap.Reserve(entry3, provider2, allowAll)

			count1 := rmap.CountReservationsForProvider(provider1)
			Expect(count1).To(Equal(2))

			count2 := rmap.CountReservationsForProvider(provider2)
			Expect(count2).To(Equal(1))
		})

		It("should return 0 for provider with no reservations", func() {
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}
			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(0))
		})
	})

	Context("TTL and expiration", func() {
		It("should expire reservations after TTL", func() {
			entryKey := client.ObjectKey{Namespace: "default", Name: "entry1"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			rmap.Reserve(entryKey, providerKey, allowAll)

			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(1))

			clock.Step(testTTL * 3 / 2)

			count = rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(0))
		})

		It("should clean up expired reservations during Reserve", func() {
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entry2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			rmap.Reserve(entry1, providerKey, allowAll)
			Expect(rmap.CountReservationsForProvider(providerKey)).To(Equal(1))

			clock.Step(testTTL * 3 / 2)

			rmap.Reserve(entry2, providerKey, allowAll)

			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(1))
		})

		It("should allow reservation check function to see current reserved keys", func() {
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entry2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			rmap.Reserve(entry1, providerKey, func(keys sets.Set[client.ObjectKey]) bool {
				Expect(keys.Len()).To(Equal(1))
				Expect(keys.Has(entry1)).To(BeTrue())
				return keys.Len() <= 2
			})

			rmap.Reserve(entry2, providerKey, func(keys sets.Set[client.ObjectKey]) bool {
				Expect(keys.Len()).To(Equal(2))
				Expect(keys.Has(entry1)).To(BeTrue())
				Expect(keys.Has(entry2)).To(BeTrue())
				return keys.Len() <= 2
			})

			finalCount := rmap.CountReservationsForProvider(providerKey)
			Expect(finalCount).To(Equal(2))
		})

		It("should reject reservation when quota would be exceeded", func() {
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entry2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			entry3 := client.ObjectKey{Namespace: "default", Name: "entry3"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			quota := 2

			rmap.Reserve(entry1, providerKey, allowUpTo(quota))
			rmap.Reserve(entry2, providerKey, allowUpTo(quota))

			success := rmap.Reserve(entry3, providerKey, allowUpTo(quota))
			Expect(success).To(BeFalse())

			count := rmap.CountReservationsForProvider(providerKey)
			Expect(count).To(Equal(2))
		})
	})

	Context("Double-counting prevention", func() {
		It("should not double-count an entry that is both provisioned and reserved", func() {
			// Simulates the race where entry-0 finished DNS provisioning (status.provider set in k8s cache)
			// but Release() hasn't been called yet, while entry-1 is checking quota concurrently.
			entry0 := client.ObjectKey{Namespace: "default", Name: "entry0"}
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}
			quota := 2

			// entry-0 holds a reservation (mid-reconcile, status already patched to k8s)
			rmap.Reserve(entry0, providerKey, allowAll)

			// entry-1 now checks quota: provisionedKeys = {entry0} (from cache), reservedEntryKeys will be {entry0, entry1}
			// Without overlap deduction: 1 + 2 = 3 > 2 → wrongly rejected
			// With overlap deduction: 1 + 2 - 1 = 2 ≤ 2 → correctly allowed
			provisionedKeys := sets.New(entry0)

			success := rmap.Reserve(entry1, providerKey, func(reservedEntryKeys sets.Set[client.ObjectKey]) bool {
				overlap := provisionedKeys.Intersection(reservedEntryKeys).Len()
				return provisionedKeys.Len()+reservedEntryKeys.Len()-overlap <= quota
			})
			Expect(success).To(BeTrue(), "entry1 should be allowed when entry0 is double-counted without deduction")
		})

		It("should correctly reject when quota is truly exceeded without double-counting", func() {
			entry0 := client.ObjectKey{Namespace: "default", Name: "entry0"}
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entry2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}
			quota := 2

			// entry-0 and entry-1 are both provisioned, no reservations
			provisionedKeys := sets.New(entry0, entry1)

			// entry-2 tries to reserve: provisioned=2, reserved=1, overlap=0 → 2+1-0=3 > 2 → rejected
			success := rmap.Reserve(entry2, providerKey, func(reservedEntryKeys sets.Set[client.ObjectKey]) bool {
				overlap := provisionedKeys.Intersection(reservedEntryKeys).Len()
				return provisionedKeys.Len()+reservedEntryKeys.Len()-overlap <= quota
			})
			Expect(success).To(BeFalse(), "entry2 should be rejected when quota is truly full")
		})
	})

	Context("Multiple providers", func() {
		It("should track reservations independently per provider", func() {
			entry1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entry2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			provider1 := client.ObjectKey{Namespace: "default", Name: "provider1"}
			provider2 := client.ObjectKey{Namespace: "default", Name: "provider2"}

			rmap.Reserve(entry1, provider1, allowAll)
			rmap.Reserve(entry2, provider2, allowAll)

			count1 := rmap.CountReservationsForProvider(provider1)
			count2 := rmap.CountReservationsForProvider(provider2)

			Expect(count1).To(Equal(1))
			Expect(count2).To(Equal(1))
		})
	})
})
