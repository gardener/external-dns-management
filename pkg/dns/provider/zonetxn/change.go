// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package zonetxn

import (
	"bytes"
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/dns"
)

type PendingTransaction struct {
	lock sync.Mutex

	zoneID             dns.ZoneID
	pendingGenerations map[client.ObjectKey]int64

	oldItems map[dns.DNSSetName][]*dns.DNSSet
	newItems dns.DNSSets
}

func NewZoneTransaction(zoneID dns.ZoneID) *PendingTransaction {
	return &PendingTransaction{
		zoneID:             zoneID,
		pendingGenerations: map[client.ObjectKey]int64{},
		oldItems:           map[dns.DNSSetName][]*dns.DNSSet{},
		newItems:           dns.DNSSets{},
	}
}

func (t *PendingTransaction) ZoneID() dns.ZoneID {
	return t.zoneID
}

func (t *PendingTransaction) AddEntryChange(key client.ObjectKey, generation int64, old, new *dns.DNSSet) {
	if old.Match(new) {
		return
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	t.pendingGenerations[key] = generation

	if old != nil {
		t.oldItems[old.Name] = append(t.oldItems[old.Name], old)
	}
	if new != nil {
		if other := t.newItems[new.Name]; other != nil {
			t.oldItems[new.Name] = append(t.oldItems[new.Name], other)
		}
		t.newItems[new.Name] = new
	}
}

func (t *PendingTransaction) AllChanges() map[dns.DNSSetName]Change {
	t.lock.Lock()
	defer t.lock.Unlock()

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

func (t *PendingTransaction) OldDNSSets() dns.DNSSets {
	t.lock.Lock()
	defer t.lock.Unlock()

	sets := dns.DNSSets{}
	for name, oldSetList := range t.oldItems {
		for _, set := range oldSetList {
			for _, recordset := range set.Sets {
				sets.AddRecordSet(name, set.RoutingPolicy, recordset)
			}
		}
	}
	return sets
}

type Change struct {
	old []*dns.DNSSet
	new *dns.DNSSet
}

func (c Change) String() string {
	var buf bytes.Buffer
	for _, old := range c.old {
		fmt.Fprintf(&buf, "old: %+v,", old)
	}
	if c.new != nil {
		fmt.Fprintf(&buf, "new: %+v", c.new)
	}
	return buf.String()
}
