// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"slices"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"
)

type dnsNameLocking struct {
	lock        sync.Mutex
	lockedNames sets.Set[string]
}

// newDNSNameLocking creates a new instance of dnsNameLocking.
func newDNSNameLocking() *dnsNameLocking {
	return &dnsNameLocking{
		lockedNames: sets.New[string](),
	}
}

// Lock locks the given DNS names. It returns true if all names were successfully locked,
// If true, they must be unlocked later using Unlock.
func (d *dnsNameLocking) Lock(names ...string) bool {
	d.lock.Lock()
	defer d.lock.Unlock()

	if slices.ContainsFunc(names, d.lockedNames.Has) {
		return false // name is already locked
	}

	for _, name := range names {
		d.lockedNames.Insert(name)
	}
	return true // Successfully locked all names
}

// Unlock unlocks the given DNS names.
func (d *dnsNameLocking) Unlock(names ...string) {
	d.lock.Lock()
	defer d.lock.Unlock()

	for _, name := range names {
		d.lockedNames.Delete(name)
	}
}
