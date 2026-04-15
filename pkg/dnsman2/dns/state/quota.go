// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"sync"
	"time"

	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// quotaReservation represents a temporary reservation of quota capacity for an entry
// that is currently being reconciled.
type quotaReservation struct {
	entryKey    client.ObjectKey
	providerKey client.ObjectKey
	timestamp   time.Time
}

// quotaReservationsMap tracks in-flight quota reservations to prevent race conditions
// where multiple concurrent reconciliations could exceed the quota.
type quotaReservationsMap struct {
	lock         sync.Mutex
	reservations map[client.ObjectKey]*quotaReservation // entryKey -> reservation
	ttl          time.Duration
	clock        clock.Clock
}

// newQuotaReservationsMap creates a new quota reservations map with the specified TTL.
func newQuotaReservationsMap(clock clock.Clock, ttl time.Duration) *quotaReservationsMap {
	return &quotaReservationsMap{
		reservations: make(map[client.ObjectKey]*quotaReservation),
		ttl:          ttl,
		clock:        clock,
	}
}

// Reserve creates a reservation for an entry with the specified provider.
// The allow function is called with the current count of reservations for the provider (including this one) and should return true if the reservation is allowed (i.e. does not exceed quota).
// Returns true if the reservation was created or already exists.
func (m *quotaReservationsMap) Reserve(entryKey, providerKey client.ObjectKey, allow func(reservedCount int32) bool) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Clean up expired reservations during reserve operation
	m.cleanupExpiredLocked()

	m.reservations[entryKey] = &quotaReservation{
		entryKey:    entryKey,
		providerKey: providerKey,
		timestamp:   m.clock.Now(),
	}

	if !allow(m.countReservationsForProvider(providerKey)) {
		delete(m.reservations, entryKey)
		return false
	}
	return true
}

// Release removes a reservation for an entry.
// Called when entry is successfully assigned or reconciliation fails.
func (m *quotaReservationsMap) Release(entryKey client.ObjectKey) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.reservations, entryKey)
}

// CountReservationsForProvider counts active (non-expired) reservations for a provider.
func (m *quotaReservationsMap) CountReservationsForProvider(providerKey client.ObjectKey) int32 {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.cleanupExpiredLocked()

	return m.countReservationsForProvider(providerKey)
}

// countReservationsForProvider counts active (non-expired) reservations for a provider. Must be called with lock held.
func (m *quotaReservationsMap) countReservationsForProvider(providerKey client.ObjectKey) int32 {
	var count int32
	for _, r := range m.reservations {
		if r.providerKey == providerKey {
			count++
		}
	}
	return count
}

// cleanupExpiredLocked removes expired reservations. Must be called with lock held.
func (m *quotaReservationsMap) cleanupExpiredLocked() {
	now := m.clock.Now()
	for k, r := range m.reservations {
		if now.Sub(r.timestamp) > m.ttl {
			delete(m.reservations, k)
		}
	}
}
