// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
	g "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dns"
)

var _ = g.Describe("entriesLocking", func() {
	var (
		locking  *entriesLocking
		entry1   resources.ObjectName
		entry2   resources.ObjectName
		entry3   resources.ObjectName
		dnsName1 = "foo1.example.com"
		dnsName2 = "foo2.example.com"
		dnsName3 = "foo3.example.com"
		zoneID   dns.ZoneID
		zoneID2  dns.ZoneID
	)

	g.BeforeEach(func() {
		locking = newEntriesLocking()
		entry1 = resources.NewObjectName("ns", "entry1")
		entry2 = resources.NewObjectName("ns", "entry2")
		entry3 = resources.NewObjectName("ns", "entry3")
		zoneID = dns.ZoneID{ProviderType: "test", ID: "zone-1"}
		zoneID2 = dns.ZoneID{ProviderType: "test", ID: "zone-2"}
	})

	g.It("should locking and unlock entry reconciliation", func() {
		ok := locking.TryLockEntryReconciliation(entry1, dnsName1)
		Expect(ok).To(BeTrue())
		ok = locking.TryLockEntryReconciliation(entry1, dnsName1)
		Expect(ok).To(BeFalse())
		locking.UnlockEntryReconciliation(entry1)
		ok = locking.TryLockEntryReconciliation(entry1, dnsName1)
		Expect(ok).To(BeTrue())
	})

	g.It("should locking zone reconciliation and return blocked entries and unlock it returning entries to trigger", func() {
		// Lock entry1 as entry
		Expect(locking.TryLockEntryReconciliation(entry1, dnsName1)).To(BeTrue())
		// Try to locking both entries for zone
		blocked := locking.TryLockZoneReconciliation(time.Now(), zoneID, "example.com", []resources.ObjectName{entry1, entry2})
		Expect(blocked).To(ContainElement(entry1))
		Expect(blocked).NotTo(ContainElement(entry2))

		locking.UnlockEntryReconciliation(entry1)
		Expect(locking.TryLockZoneReconciliation(time.Now(), zoneID, "example.com", blocked)).To(BeEmpty(), "should not block any entries after unlocking entry1")
		Expect(locking.ongoingZoneReconciliations).To(HaveLen(1))

		Expect(locking.TryLockEntryReconciliation(entry1, dnsName1)).To(BeFalse())

		Expect(locking.TryLockEntryReconciliation(entry3, "other.example2.com")).To(BeTrue(), "entry3 should not be blocked by zone locking")
		locking.UnlockEntryReconciliation(entry3)

		Expect(locking.TryLockEntryReconciliation(entry3, dnsName3)).To(BeFalse(), "entry3 should be blocked because of matching domain name")

		toBeTriggered := locking.UnlockZoneReconciliation(zoneID)
		Expect(toBeTriggered).To(ConsistOf(entry1, entry3))

		Expect(locking.ongoingZoneReconciliations).To(BeEmpty(), "should not have any ongoing zone reconciliations after unlocking")
	})

	g.It("should unlock zone reconciliation and trigger entries", func() {
		locking.TryLockZoneReconciliation(time.Now(), zoneID, "example.com", []resources.ObjectName{entry1, entry2})
		// Simulate entry2 was requested while locked
		Expect(locking.TryLockEntryReconciliation(entry2, dnsName2)).To(BeFalse(), "entry2 should be blocked because of zone locking")
		Expect(locking.TryLockEntryReconciliation(entry3, "foo3.example2.com")).To(BeTrue())

		// Unlock zone, should trigger entry2
		triggered := locking.UnlockZoneReconciliation(zoneID)
		Expect(triggered).To(ConsistOf(entry2))
	})

	g.It("should unlock zone reconciliations and trigger entries only after last matching zone", func() {
		locking.TryLockZoneReconciliation(time.Now(), zoneID, "example.com", []resources.ObjectName{entry1, entry2})
		// assume second private zone with the same domain name
		locking.TryLockZoneReconciliation(time.Now(), zoneID2, "example.com", []resources.ObjectName{})
		// Simulate entry2 was requested while locked
		Expect(locking.TryLockEntryReconciliation(entry2, dnsName2)).To(BeFalse(), "entry2 should be blocked because of zone locking")

		// Unlock zone, should trigger entry2
		triggered := locking.UnlockZoneReconciliation(zoneID)
		Expect(triggered).To(BeEmpty())
		triggered = locking.UnlockZoneReconciliation(zoneID2)
		Expect(triggered).To(ConsistOf(entry2))
	})
})
