// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/clock"
	"k8s.io/utils/clock/testing"
)

var _ = Describe("dnsNameLocking", func() {
	var (
		fakeClock *testing.FakeClock
		locking   *dnsNameLocking
	)

	BeforeEach(func() {
		fakeClock = testing.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		locking = newDNSNameLocking(fakeClock)
	})

	Describe("Lock", func() {
		It("locks free names and returns (true, 0)", func() {
			ok, retry := locking.Lock("a.example.com", "b.example.com")
			Expect(ok).To(BeTrue())
			Expect(retry).To(Equal(time.Duration(0)))
		})

		It("returns (false, 0) when blocked by an in-progress holder", func() {
			ok, _ := locking.Lock("a.example.com")
			Expect(ok).To(BeTrue())

			ok2, retry := locking.Lock("a.example.com")
			Expect(ok2).To(BeFalse())
			Expect(retry).To(Equal(time.Duration(0)))
		})

		It("fails if any one of multiple names is held in-progress", func() {
			ok, _ := locking.Lock("a.example.com")
			Expect(ok).To(BeTrue())

			ok2, retry := locking.Lock("b.example.com", "a.example.com", "c.example.com")
			Expect(ok2).To(BeFalse())
			Expect(retry).To(Equal(time.Duration(0)))

			// none of the unrelated names should have been claimed by the failed call
			ok3, _ := locking.Lock("b.example.com")
			Expect(ok3).To(BeTrue())
			ok4, _ := locking.Lock("c.example.com")
			Expect(ok4).To(BeTrue())
		})

		It("returns retryAfter equal to the remaining reservation", func() {
			ok, _ := locking.Lock("a.example.com")
			Expect(ok).To(BeTrue())
			locking.UnlockWithExpiry(200*time.Millisecond, "a.example.com")

			ok2, retry := locking.Lock("a.example.com")
			Expect(ok2).To(BeFalse())
			Expect(retry).To(Equal(200 * time.Millisecond))

			fakeClock.Step(50 * time.Millisecond)
			ok3, retry2 := locking.Lock("a.example.com")
			Expect(ok3).To(BeFalse())
			Expect(retry2).To(Equal(150 * time.Millisecond))
		})

		It("returns the largest retryAfter across multiple reserved names", func() {
			ok, _ := locking.Lock("a.example.com", "b.example.com")
			Expect(ok).To(BeTrue())
			locking.UnlockWithExpiry(50*time.Millisecond, "a.example.com")
			locking.UnlockWithExpiry(300*time.Millisecond, "b.example.com")

			ok2, retry := locking.Lock("a.example.com", "b.example.com")
			Expect(ok2).To(BeFalse())
			Expect(retry).To(Equal(300 * time.Millisecond))
		})

		It("succeeds once all reservations have expired", func() {
			ok, _ := locking.Lock("a.example.com")
			Expect(ok).To(BeTrue())
			locking.UnlockWithExpiry(100*time.Millisecond, "a.example.com")

			ok2, _ := locking.Lock("a.example.com")
			Expect(ok2).To(BeFalse())

			fakeClock.Step(100 * time.Millisecond)
			ok3, retry := locking.Lock("a.example.com")
			Expect(ok3).To(BeTrue())
			Expect(retry).To(Equal(time.Duration(0)))
		})

		It("supports re-locking after Unlock", func() {
			ok, _ := locking.Lock("a.example.com")
			Expect(ok).To(BeTrue())
			locking.Unlock("a.example.com")

			ok2, retry := locking.Lock("a.example.com")
			Expect(ok2).To(BeTrue())
			Expect(retry).To(Equal(time.Duration(0)))
		})

		It("treats a no-name call as a no-op success", func() {
			ok, retry := locking.Lock()
			Expect(ok).To(BeTrue())
			Expect(retry).To(Equal(time.Duration(0)))
		})

		It("prefers the in-progress signal (retry=0) when the request is blocked by both an in-progress holder and a reservation", func() {
			// reserve "a" with a long expiry, hold "b" in-progress
			ok, _ := locking.Lock("a.example.com")
			Expect(ok).To(BeTrue())
			locking.UnlockWithExpiry(time.Hour, "a.example.com")

			ok2, _ := locking.Lock("b.example.com")
			Expect(ok2).To(BeTrue())

			// blocked by both: in-progress holder wins -> retry=0
			ok3, retry := locking.Lock("a.example.com", "b.example.com")
			Expect(ok3).To(BeFalse())
			Expect(retry).To(Equal(time.Duration(0)))
		})

		It("does not claim any names when contended", func() {
			ok, _ := locking.Lock("held.example.com")
			Expect(ok).To(BeTrue())

			ok2, _ := locking.Lock("free.example.com", "held.example.com")
			Expect(ok2).To(BeFalse())

			// "free" must still be acquirable by someone else
			ok3, _ := locking.Lock("free.example.com")
			Expect(ok3).To(BeTrue())
		})
	})

	Describe("Unlock", func() {
		It("clears in-progress holds", func() {
			ok, _ := locking.Lock("a.example.com", "b.example.com")
			Expect(ok).To(BeTrue())
			locking.Unlock("a.example.com", "b.example.com")

			ok2, _ := locking.Lock("a.example.com", "b.example.com")
			Expect(ok2).To(BeTrue())
		})

		It("is a no-op for unknown names", func() {
			Expect(func() { locking.Unlock("never-locked.example.com") }).NotTo(Panic())
		})

		It("does not affect names other than those passed", func() {
			ok, _ := locking.Lock("a.example.com", "b.example.com")
			Expect(ok).To(BeTrue())
			locking.Unlock("a.example.com")

			ok2, retry := locking.Lock("b.example.com")
			Expect(ok2).To(BeFalse())
			Expect(retry).To(Equal(time.Duration(0)))
		})

		It("clears an active reservation", func() {
			locking.UnlockWithExpiry(time.Hour, "a.example.com")

			ok, _ := locking.Lock("a.example.com")
			Expect(ok).To(BeFalse())

			locking.Unlock("a.example.com")
			ok2, _ := locking.Lock("a.example.com")
			Expect(ok2).To(BeTrue())
		})
	})

	Describe("UnlockWithExpiry", func() {
		It("falls back to immediate Unlock for non-positive durations", func() {
			ok, _ := locking.Lock("a.example.com")
			Expect(ok).To(BeTrue())
			locking.UnlockWithExpiry(0, "a.example.com")

			ok2, _ := locking.Lock("a.example.com")
			Expect(ok2).To(BeTrue())

			locking.UnlockWithExpiry(-5*time.Second, "a.example.com")
			ok3, _ := locking.Lock("a.example.com")
			Expect(ok3).To(BeTrue())
		})

		It("blocks subsequent locks until the reservation expires", func() {
			ok, _ := locking.Lock("a.example.com")
			Expect(ok).To(BeTrue())
			locking.UnlockWithExpiry(50*time.Millisecond, "a.example.com")

			ok2, retry := locking.Lock("a.example.com")
			Expect(ok2).To(BeFalse())
			Expect(retry).To(Equal(50 * time.Millisecond))

			fakeClock.Step(50 * time.Millisecond)
			ok3, _ := locking.Lock("a.example.com")
			Expect(ok3).To(BeTrue())
		})

		It("can be called on names that were not previously held", func() {
			locking.UnlockWithExpiry(100*time.Millisecond, "fresh.example.com")

			ok, retry := locking.Lock("fresh.example.com")
			Expect(ok).To(BeFalse())
			Expect(retry).To(Equal(100 * time.Millisecond))
		})

		It("overwrites a prior reservation with the new expiry", func() {
			locking.UnlockWithExpiry(time.Second, "a.example.com")
			locking.UnlockWithExpiry(10*time.Millisecond, "a.example.com")

			_, retry := locking.Lock("a.example.com")
			Expect(retry).To(Equal(10 * time.Millisecond))
		})
	})

	Describe("concurrency", func() {
		It("permits exactly one in-progress holder of the same name at any instant", func() {
			// Use a real clock so multiple goroutines see real time advancing.
			real := newDNSNameLocking(clock.RealClock{})

			const goroutines = 50
			var wg sync.WaitGroup
			var mu sync.Mutex
			var inFlight, maxInFlight int

			wg.Add(goroutines)
			for range goroutines {
				go func() {
					defer wg.Done()
					if ok, _ := real.Lock("contested.example.com"); ok {
						mu.Lock()
						inFlight++
						if inFlight > maxInFlight {
							maxInFlight = inFlight
						}
						mu.Unlock()
						time.Sleep(100 * time.Microsecond)
						mu.Lock()
						inFlight--
						mu.Unlock()
						real.Unlock("contested.example.com")
					}
				}()
			}
			wg.Wait()

			Expect(maxInFlight).To(Equal(1))
			ok, _ := real.Lock("contested.example.com")
			Expect(ok).To(BeTrue())
		})
	})
})
