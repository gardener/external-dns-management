// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package records

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

// helpers to build ChangeRequests concisely
func createCR(dnsName, setID string) *provider.ChangeRequests {
	name := dns.DNSSetName{DNSName: dnsName, SetIdentifier: setID}
	cr := provider.NewChangeRequests(name)
	cr.Updates[dns.TypeA] = &provider.ChangeRequestUpdate{
		Old: nil,
		New: &dns.RecordSet{Type: dns.TypeA},
	}
	return cr
}

func deleteCR(dnsName, setID string) *provider.ChangeRequests {
	name := dns.DNSSetName{DNSName: dnsName, SetIdentifier: setID}
	cr := provider.NewChangeRequests(name)
	cr.Updates[dns.TypeA] = &provider.ChangeRequestUpdate{
		Old: &dns.RecordSet{Type: dns.TypeA},
		New: nil,
	}
	return cr
}

var _ = Describe("orderChanges", func() {
	plain := func(dnsName string) dns.DNSSetName {
		return dns.DNSSetName{DNSName: dnsName}
	}
	policy := func(dnsName, id string) dns.DNSSetName {
		return dns.DNSSetName{DNSName: dnsName, SetIdentifier: id}
	}

	DescribeTable("ordering change requests",
		func(input map[dns.DNSSetName]*provider.ChangeRequests, assertFn func([]dns.DNSSetName)) {
			result := orderChanges(input)
			assertFn(result)
		},

		Entry("empty input returns empty slice",
			map[dns.DNSSetName]*provider.ChangeRequests{},
			func(keys []dns.DNSSetName) {
				Expect(keys).To(BeEmpty())
			},
		),

		Entry("only plain keys are returned as-is",
			map[dns.DNSSetName]*provider.ChangeRequests{
				plain("a.example.com"): createCR("a.example.com", ""),
				plain("b.example.com"): deleteCR("b.example.com", ""),
			},
			func(keys []dns.DNSSetName) {
				Expect(keys).To(ConsistOf(plain("a.example.com"), plain("b.example.com")))
			},
		),

		Entry("only policy keys are returned as-is",
			map[dns.DNSSetName]*provider.ChangeRequests{
				policy("a.example.com", "p1"): createCR("a.example.com", "p1"),
				policy("a.example.com", "p2"): deleteCR("a.example.com", "p2"),
			},
			func(keys []dns.DNSSetName) {
				Expect(keys).To(ConsistOf(policy("a.example.com", "p1"), policy("a.example.com", "p2")))
			},
		),

		Entry("plain create + policy delete for same name: policy delete comes before plain create",
			map[dns.DNSSetName]*provider.ChangeRequests{
				plain("a.example.com"):        createCR("a.example.com", ""),
				policy("a.example.com", "p1"): deleteCR("a.example.com", "p1"),
			},
			func(keys []dns.DNSSetName) {
				Expect(keys).To(HaveLen(2))
				Expect(keys[0]).To(Equal(policy("a.example.com", "p1")))
				Expect(keys[1]).To(Equal(plain("a.example.com")))
			},
		),

		Entry("plain delete + policy create for same name: no special reordering (plain before policy)",
			map[dns.DNSSetName]*provider.ChangeRequests{
				plain("a.example.com"):        deleteCR("a.example.com", ""),
				policy("a.example.com", "p1"): createCR("a.example.com", "p1"),
			},
			func(keys []dns.DNSSetName) {
				Expect(keys).To(ConsistOf(plain("a.example.com"), policy("a.example.com", "p1")))
			},
		),

		Entry("plain create + policy create for same name: both present, no special ordering needed",
			map[dns.DNSSetName]*provider.ChangeRequests{
				plain("a.example.com"):        createCR("a.example.com", ""),
				policy("a.example.com", "p1"): createCR("a.example.com", "p1"),
			},
			func(keys []dns.DNSSetName) {
				Expect(keys).To(ConsistOf(plain("a.example.com"), policy("a.example.com", "p1")))
			},
		),

		Entry("plain and policy keys for different names: no special interleaving",
			map[dns.DNSSetName]*provider.ChangeRequests{
				plain("a.example.com"):        createCR("a.example.com", ""),
				policy("b.example.com", "p1"): deleteCR("b.example.com", "p1"),
			},
			func(keys []dns.DNSSetName) {
				Expect(keys).To(ConsistOf(plain("a.example.com"), policy("b.example.com", "p1")))
			},
		),

		Entry("multiple policy keys for same plain name: only the deletion policy key is front-ordered",
			map[dns.DNSSetName]*provider.ChangeRequests{
				plain("a.example.com"):        createCR("a.example.com", ""),
				policy("a.example.com", "p1"): deleteCR("a.example.com", "p1"),
				policy("a.example.com", "p2"): createCR("a.example.com", "p2"),
			},
			func(keys []dns.DNSSetName) {
				Expect(keys).To(HaveLen(3))
				Expect(keys[0]).To(Equal(policy("a.example.com", "p1")))
				// plain and p2 follow in any order
				Expect(keys[1:]).To(ConsistOf(plain("a.example.com"), policy("a.example.com", "p2")))
			},
		),
	)
})

var _ = Describe("isDeletion", func() {
	DescribeTable("detecting deletion change requests",
		func(cr *provider.ChangeRequests, expected bool) {
			Expect(isDeletion(cr)).To(Equal(expected))
		},

		Entry("all updates are deletions (New == nil)",
			&provider.ChangeRequests{
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {Old: &dns.RecordSet{}, New: nil},
				},
			},
			true,
		),

		Entry("all updates are creates (New != nil)",
			&provider.ChangeRequests{
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {Old: nil, New: &dns.RecordSet{}},
				},
			},
			false,
		),

		Entry("mixed updates: at least one deletion returns true",
			&provider.ChangeRequests{
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA:    {Old: &dns.RecordSet{}, New: nil},
					dns.TypeAAAA: {Old: nil, New: &dns.RecordSet{}},
				},
			},
			true,
		),

		Entry("update (Old and New both set) returns false",
			&provider.ChangeRequests{
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {Old: &dns.RecordSet{}, New: &dns.RecordSet{}},
				},
			},
			false,
		),

		Entry("empty updates returns false",
			&provider.ChangeRequests{
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{},
			},
			false,
		),
	)
})
