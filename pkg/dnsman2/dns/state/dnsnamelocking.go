// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"sync"
	"time"

	"k8s.io/utils/clock"
)

type dnsNameLocking struct {
	clock clock.Clock
	lock  sync.Mutex
	// lockedNames maps a DNS name to its lock expiry. A zero time means the name is
	// actively held by an in-progress reconciliation; a non-zero time means the name
	// is reserved until that instant (e.g. for DNS propagation) but no goroutine is
	// currently holding it.
	lockedNames map[string]time.Time
}

// newDNSNameLocking creates a new instance of dnsNameLocking using the given clock.
func newDNSNameLocking(c clock.Clock) *dnsNameLocking {
	return &dnsNameLocking{
		clock:       c,
		lockedNames: map[string]time.Time{},
	}
}

// Lock attempts to lock the given DNS names for an in-progress reconciliation.
// On success it returns (true, 0) and the caller must release the names later
// via Unlock or UnlockWithExpiry.
// On contention it returns (false, retryAfter) where retryAfter is the time
// until the latest blocking reservation is expected to expire (or 0 if blocked
// by another in-progress holder).
func (d *dnsNameLocking) Lock(names ...string) (bool, time.Duration) {
	d.lock.Lock()
	defer d.lock.Unlock()

	now := d.clock.Now()
	var retryAfter time.Duration
	for _, name := range names {
		exp, ok := d.lockedNames[name]
		if !ok {
			continue
		}
		if exp.IsZero() {
			// in-progress holder; no known release time
			return false, 0
		}
		if exp.After(now) {
			if remaining := exp.Sub(now); remaining > retryAfter {
				retryAfter = remaining
			}
			continue
		}
		// expired reservation, treat as free
	}
	if retryAfter > 0 {
		return false, retryAfter
	}

	for _, name := range names {
		d.lockedNames[name] = time.Time{}
	}
	return true, 0
}

// Unlock releases the given DNS names immediately.
func (d *dnsNameLocking) Unlock(names ...string) {
	d.lock.Lock()
	defer d.lock.Unlock()

	for _, name := range names {
		delete(d.lockedNames, name)
	}
}

// UnlockWithExpiry releases the given DNS names but keeps them reserved for
// the given duration so that subsequent Lock attempts on the same names fail
// (and requeue) until the reservation expires. Use this after a successful
// DNS change to gate other reconciliations until the change has propagated.
// A non-positive duration behaves like Unlock.
func (d *dnsNameLocking) UnlockWithExpiry(reservation time.Duration, names ...string) {
	if reservation <= 0 {
		d.Unlock(names...)
		return
	}
	d.lock.Lock()
	defer d.lock.Unlock()

	expiry := d.clock.Now().Add(reservation)
	for _, name := range names {
		d.lockedNames[name] = expiry
	}
}
