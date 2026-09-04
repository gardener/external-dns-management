// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	g "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/zonetxn"
)

var _ = g.Describe("cleanupEntry backend deletion guard", func() {
	// Regression test for foreign/stale DNS entries being deleted ~15 minutes
	// after the DNSEntry object is removed:
	//
	// When EntryDeleted runs while no valid provider currently resolves a zone
	// for the entry ("removing foreign entry"), cleanupEntry must NOT schedule a
	// backend delete into the zone transaction. Otherwise the record - which is
	// otherwise preserved as stale - gets orphaned and deleted on the next zone
	// reconcile ("found unapplied managed set" -> DELETE).

	g.Describe("cleanupInBackend", func() {
		g.It("deletes only when the entry is managed and a zone is currently resolvable", func() {
			Expect(cleanupInBackend(false, true)).To(BeTrue(), "managed entry with resolvable zone -> delete")

			// the regression: provider temporarily invalid, no zone resolvable
			Expect(cleanupInBackend(false, false)).To(BeFalse(), "no resolvable zone -> preserve (foreign/stale)")

			// obsolete = handled only by a fallback provider
			Expect(cleanupInBackend(true, true)).To(BeFalse(), "obsolete entry -> preserve")
			Expect(cleanupInBackend(true, false)).To(BeFalse(), "obsolete entry -> preserve")
		})
	})

	g.Describe("effect on the pending zone transaction", func() {
		var (
			zoneID  dns.ZoneID
			setName dns.DNSSetName
			key     client.ObjectKey
			oldSet  *dns.DNSSet
		)

		g.BeforeEach(func() {
			zoneID = dns.NewZoneID("aws-route53", "Z02026782CY0WR1NYZD7R")
			setName = dns.DNSSetName{DNSName: "test-dns.example.com"}
			key = client.ObjectKey{Namespace: "ns", Name: "smoke-test-dns-entry"}
			oldSet = dns.NewDNSSet(setName, nil)
			oldSet.SetRecordSet(dns.RS_CNAME, 300, "2026-09-01-13-09-28UTC.build23878441")
		})

		// applyCleanup mirrors the decision cleanupEntry makes for the active
		// zone transaction (see state_entry.go): only when cleanupInBackend
		// allows it, the old record set is queued for deletion (old -> nil).
		applyCleanup := func(obsolete, zoneResolvable bool) *zonetxn.PendingTransaction {
			txn := zonetxn.NewZoneTransaction(zoneID)
			if cleanupInBackend(obsolete, zoneResolvable) {
				txn.AddEntryChange(key, 1, oldSet, nil)
			}
			return txn
		}

		g.It("queues the record for deletion when a zone is resolvable", func() {
			txn := applyCleanup(false, true)
			Expect(txn.OldDNSSets()).To(HaveKey(setName))
		})

		g.It("does NOT queue the record for deletion for a foreign/stale entry (no resolvable zone)", func() {
			txn := applyCleanup(false, false)
			Expect(txn.OldDNSSets()).To(BeEmpty())
		})

		g.It("does NOT queue the record for deletion for an obsolete entry", func() {
			txn := applyCleanup(true, true)
			Expect(txn.OldDNSSets()).To(BeEmpty())
		})
	})
})
