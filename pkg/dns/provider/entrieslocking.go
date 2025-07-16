// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"

	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type entriesLocking struct {
	lock sync.Mutex

	lockedEntries              map[resources.ObjectName]origin
	entriesToTrigger           map[resources.ObjectName]struct{}
	ongoingZoneReconciliations map[dns.ZoneID]string
	outstandingEntries         map[resources.ObjectName]time.Time
}

type originType string

const (
	originTypeEntry originType = "entry"
	originTypeZone  originType = "zone"

	outstandingEntriesTimeout = 15 * time.Second
)

type origin struct {
	originType originType
	dnsName    string // DNS name of the entry, if originType is entry
	zoneIDs    []dns.ZoneID
}

func newEntriesLocking() *entriesLocking {
	return &entriesLocking{
		lockedEntries:              make(map[resources.ObjectName]origin),
		entriesToTrigger:           make(map[resources.ObjectName]struct{}),
		ongoingZoneReconciliations: make(map[dns.ZoneID]string),
		outstandingEntries:         make(map[resources.ObjectName]time.Time),
	}
}

// TryLockEntryReconciliation attempts to lock an entry for reconciliation.
func (l *entriesLocking) TryLockEntryReconciliation(entry resources.ObjectName, dnsName string) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	for zoneID, zoneDomain := range l.ongoingZoneReconciliations {
		if dnsutils.Match(dns.NormalizeHostname(dnsName), zoneDomain) {
			// The entry is part of an ongoing zone reconciliation
			l.lockedEntries[entry] = addZone(l.lockedEntries[entry], zoneID)
		}
	}

	if _, exists := l.lockedEntries[entry]; exists {
		l.entriesToTrigger[entry] = struct{}{}
		return false
	}

	delete(l.outstandingEntries, entry)

	l.lockedEntries[entry] = origin{
		originType: originTypeEntry,
		dnsName:    dns.NormalizeHostname(dnsName),
	}
	return true
}

// UnlockEntryReconciliation unlocks an entry that was locked for reconciliation.
func (l *entriesLocking) UnlockEntryReconciliation(entry resources.ObjectName) {
	l.lock.Lock()
	defer l.lock.Unlock()

	delete(l.lockedEntries, entry)
}

// TryLockZoneReconciliation attempts to lock a set of entries for zone reconciliation.
// It returns a slice of entries that could not be locked because they are already in use.
// Note that the other entries are locked. It is expected that this method is repeatedly called,
// until all entries are locked. UnlockZoneReconciliation must always be called for the zoneID.
func (l *entriesLocking) TryLockZoneReconciliation(startTime time.Time, zoneID dns.ZoneID, zoneDomain string, entries []resources.ObjectName) []resources.ObjectName {
	l.lock.Lock()
	defer l.lock.Unlock()

	var blockedEntries []resources.ObjectName
	for _, entry := range entries {
		if t, exists := l.outstandingEntries[entry]; exists && startTime.Sub(t) < outstandingEntriesTimeout {
			// If the entry is still outstanding, let's wait a bit longer
			blockedEntries = append(blockedEntries, entry)
		}
	}
	if len(blockedEntries) > 0 {
		return blockedEntries
	}

	for _, entry := range entries {
		o, exists := l.lockedEntries[entry]
		if exists && o.originType == originTypeEntry {
			blockedEntries = append(blockedEntries, entry)
			continue
		}
		o.originType = originTypeZone
		l.lockedEntries[entry] = addZone(o, zoneID)
	}

	l.ongoingZoneReconciliations[zoneID] = zoneDomain
outer:
	for entry, o := range l.lockedEntries {
		if o.originType == originTypeEntry && dnsutils.Match(o.dnsName, zoneDomain) {
			for _, blockedEntry := range blockedEntries {
				if blockedEntry == entry {
					// If the entry is already blocked, we do not need to add it again
					continue outer
				}
			}
			blockedEntries = append(blockedEntries, entry)
		}
	}

	return blockedEntries
}

// UnlockZoneReconciliation unlocks all entries that were locked for the given zoneID and returns a slice of entries that
// need to be triggered for reconciliation.
func (l *entriesLocking) UnlockZoneReconciliation(zoneID dns.ZoneID) []resources.ObjectName {
	l.lock.Lock()
	defer l.lock.Unlock()

	var toBeTriggered []resources.ObjectName
	for entry, o := range l.lockedEntries {
		if o.originType == originTypeZone {
			if len(o.zoneIDs) == 1 && o.zoneIDs[0] == zoneID {
				delete(l.lockedEntries, entry)
				if _, exists := l.entriesToTrigger[entry]; exists {
					toBeTriggered = append(toBeTriggered, entry)
					l.outstandingEntries[entry] = time.Now()
					delete(l.entriesToTrigger, entry)
				}
			} else {
				// Remove only the specific zoneID from the zoneIDs list
				var newZoneIDs []dns.ZoneID
				for _, existingZoneID := range o.zoneIDs {
					if existingZoneID != zoneID {
						newZoneIDs = append(newZoneIDs, existingZoneID)
					}
				}
				o.zoneIDs = newZoneIDs
				l.lockedEntries[entry] = o
			}
		}
	}

	delete(l.ongoingZoneReconciliations, zoneID)

	// Clean up outstanding entries that are older than timeout
	for entry, timestamp := range l.outstandingEntries {
		if time.Since(timestamp) > outstandingEntriesTimeout {
			delete(l.outstandingEntries, entry)
		}
	}

	return toBeTriggered
}

func addZone(o origin, zoneID dns.ZoneID) origin {
	o.originType = originTypeZone
	if o.zoneIDs == nil {
		o.zoneIDs = []dns.ZoneID{}
	}
	for _, existingZoneID := range o.zoneIDs {
		if existingZoneID == zoneID {
			return o // Zone already exists, no need to add it again
		}
	}
	o.zoneIDs = append(o.zoneIDs, zoneID)
	return o
}
