// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("entryFailureBackoff", func() {
	var (
		backoff   *entryFailureBackoff
		fakeClock *testing.FakeClock
		key       client.ObjectKey
		fp        entryFingerprint
	)

	BeforeEach(func() {
		fakeClock = testing.NewFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		backoff = newEntryFailureBackoff(entryFailureBackoffConfig{
			base:   30 * time.Second,
			factor: 2,
			max:    10 * time.Minute,
		}, fakeClock)
		key = client.ObjectKey{Namespace: "test", Name: "entry1"}
		fp = computeEntryFingerprint(1, map[string]string{"dns.gardener.cloud/class": "gardendns"})
	})

	It("reports no block for unknown entries", func() {
		_, blocked := backoff.blockedUntilFor(key, fp)
		Expect(blocked).To(BeFalse())
	})

	It("blocks after a failure and unblocks once the window elapsed", func() {
		next := backoff.recordFailureFor(key, fp)
		Expect(next).To(BeTemporally(">", fakeClock.Now()))

		until, blocked := backoff.blockedUntilFor(key, fp)
		Expect(blocked).To(BeTrue())
		Expect(until).To(Equal(next))

		fakeClock.SetTime(next)
		_, blocked = backoff.blockedUntilFor(key, fp)
		Expect(blocked).To(BeFalse())
	})

	It("does not block when the generation changed", func() {
		backoff.recordFailureFor(key, fp)

		changed := computeEntryFingerprint(2, map[string]string{"dns.gardener.cloud/class": "gardendns"})
		_, blocked := backoff.blockedUntilFor(key, changed)
		Expect(blocked).To(BeFalse())
	})

	It("does not block when a dns.gardener.cloud annotation changed", func() {
		backoff.recordFailureFor(key, fp)

		changed := computeEntryFingerprint(1, map[string]string{"dns.gardener.cloud/class": "other"})
		_, blocked := backoff.blockedUntilFor(key, changed)
		Expect(blocked).To(BeFalse())
	})

	It("ignores annotations outside the dns.gardener.cloud group", func() {
		backoff.recordFailureFor(key, fp)

		// adding an unrelated annotation must not reset the backoff
		same := computeEntryFingerprint(1, map[string]string{
			"dns.gardener.cloud/class":   "gardendns",
			"other.example.com/whatever": "x",
		})
		_, blocked := backoff.blockedUntilFor(key, same)
		Expect(blocked).To(BeTrue())
	})

	It("restarts the backoff count when the entry changed", func() {
		backoff.recordFailureFor(key, fp)
		first := backoff.recordFailureFor(key, fp) // count == 2

		changed := computeEntryFingerprint(2, map[string]string{"dns.gardener.cloud/class": "gardendns"})
		after := backoff.recordFailureFor(key, changed) // count reset to 1

		// count reset -> window back to base (<= base+jitter), shorter than the count==2 window
		base := float64(30 * time.Second)
		Expect(after.Sub(fakeClock.Now())).To(BeNumerically("<=", time.Duration(base*1.1)))
		Expect(after.Sub(fakeClock.Now())).To(BeNumerically("<", first.Sub(fakeClock.Now())))
	})

	It("grows the backoff duration with consecutive failures", func() {
		var prev time.Duration
		for i := 1; i <= 6; i++ {
			d := backoff.backoffDuration(i)
			if i > 1 {
				Expect(d).To(BeNumerically(">", prev))
			}
			prev = d
		}
	})

	It("caps the backoff duration at max (accounting for jitter)", func() {
		// large count -> capped at max, plus up to ±10% jitter
		maxDur := float64(10 * time.Minute)
		maxUpper := time.Duration(maxDur * 1.1)
		maxLower := time.Duration(maxDur * 0.9)
		d := backoff.backoffDuration(100)
		Expect(d).To(BeNumerically("<=", maxUpper))
		Expect(d).To(BeNumerically(">=", maxLower))
	})

	It("applies jitter within ±10% of the base for the first failure", func() {
		base := float64(30 * time.Second)
		lower := time.Duration(base * 0.9)
		upper := time.Duration(base * 1.1)
		for range 50 {
			d := backoff.backoffDuration(1)
			Expect(d).To(BeNumerically(">=", lower))
			Expect(d).To(BeNumerically("<=", upper))
		}
	})

	It("resets the counter on clear", func() {
		backoff.recordFailureFor(key, fp)
		backoff.recordFailureFor(key, fp)
		backoff.clear(key)

		_, blocked := backoff.blockedUntilFor(key, fp)
		Expect(blocked).To(BeFalse())

		// after clear, next failure starts again at the base window
		base := float64(30 * time.Second)
		upper := time.Duration(base * 1.1)
		next := backoff.recordFailureFor(key, fp)
		Expect(next.Sub(fakeClock.Now())).To(BeNumerically("<=", upper))
	})
})
