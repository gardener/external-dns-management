/*
 * // SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 * //
 * // SPDX-License-Identifier: Apache-2.0
 */

package zonetxn

import (
	"sync"

	"github.com/gardener/external-dns-management/pkg/dns"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ZoneTransaction struct {
	lock sync.Mutex

	zoneID dns.ZoneID
	goals  map[client.ObjectKey]*dns.DNSSet

	oldItems map[dns.DNSSetName][]*dns.DNSSet
	newItems dns.DNSSets
}

func NewZoneTransaction(zoneID dns.ZoneID) *ZoneTransaction {
	return &ZoneTransaction{
		zoneID:   zoneID,
		goals:    map[client.ObjectKey]*dns.DNSSet{},
		oldItems: map[dns.DNSSetName][]*dns.DNSSet{},
		newItems: dns.DNSSets{},
	}
}

func (t *ZoneTransaction) ZoneID() dns.ZoneID {
	return t.zoneID
}

func (t *ZoneTransaction) AddEntryChange(key client.ObjectKey, old, new *dns.DNSSet) {
	if old.Match(new) {
		return
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	t.goals[key] = new

	if old != nil {
		t.oldItems[old.Name] = append(t.oldItems[new.Name], old)
	}
	if new != nil {
		if other := t.newItems[new.Name]; other != nil {
			t.oldItems[new.Name] = append(t.oldItems[new.Name], other)
		}
		t.newItems[new.Name] = new
	}
}

func (t *ZoneTransaction) AllChanges() map[dns.DNSSetName]Change {
	changes := map[dns.DNSSetName]Change{}
	for name, oldSets := range t.oldItems {
		change := Change{
			old: oldSets,
		}
		change.new = t.newItems[name]
		changes[name] = change
	}
	for name, newSet := range t.newItems {
		if _, found := changes[name]; !found {
			changes[name] = Change{new: newSet}
		}
	}
	return changes
}

type Change struct {
	old []*dns.DNSSet
	new *dns.DNSSet
}
