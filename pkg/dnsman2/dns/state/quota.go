// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// quotaExceededEntriesMap is a thread-safe map that tracks DNSEntry resources
// that have been blocked due to provider entries quota being exceeded.
// It maps entry keys to their intended provider keys for efficient lookups
// when quotas are increased or entries are deleted.
type quotaExceededEntriesMap struct {
	lock    sync.Mutex
	entries map[client.ObjectKey]client.ObjectKey // entryKey -> providerKey
}

// newQuotaExceededEntriesMap creates a new empty quotaExceededEntriesMap.
func newQuotaExceededEntriesMap() *quotaExceededEntriesMap {
	return &quotaExceededEntriesMap{
		entries: make(map[client.ObjectKey]client.ObjectKey),
	}
}

// Add tracks an entry that was blocked due to quota being exceeded for a provider.
func (m *quotaExceededEntriesMap) Add(entryKey, providerKey client.ObjectKey) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.entries[entryKey] = providerKey
}

// Remove stops tracking an entry (called when entry is assigned, deleted, or no longer quota-blocked).
func (m *quotaExceededEntriesMap) Remove(entryKey client.ObjectKey) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.entries, entryKey)
}

// GetProvider returns the provider key for a quota-blocked entry, or nil if not tracked.
func (m *quotaExceededEntriesMap) GetProvider(entryKey client.ObjectKey) *client.ObjectKey {
	m.lock.Lock()
	defer m.lock.Unlock()

	if providerKey, ok := m.entries[entryKey]; ok {
		return &providerKey
	}
	return nil
}
