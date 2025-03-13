// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("DNSSet", func() {
	Describe("Clone", func() {
		It("should clone DNSSet correctly", func() {
			original := &DNSSet{
				Name:        DNSSetName{DNSName: "example.com"},
				UpdateGroup: "group1",
				Sets: RecordSets{
					"A": &RecordSet{Type: "A", TTL: 300, Records: []*Record{{Value: "1.2.3.4"}}},
				},
				RoutingPolicy: &RoutingPolicy{Type: RoutingPolicyWeighted},
			}

			other := NewDNSSet(DNSSetName{DNSName: "example.com"}, &RoutingPolicy{Type: RoutingPolicyWeighted})
			other.SetRecordSet(TypeA, 300, "1.2.3.4")
			other.UpdateGroup = "group1"
			Expect(other).To(Equal(original))
			Expect(other).ToNot(BeIdenticalTo(original))
		})
	})

	Describe("SetRecordSet", func() {
		It("should set record set correctly", func() {
			dnsSet := &DNSSet{Sets: RecordSets{}}
			dnsSet.SetRecordSet(TypeA, 300, "1.2.3.4", "5.6.7.8")
			dnsSet.SetRecordSet(TypeAAAA, 100, "::1")
			Expect(dnsSet.Sets["A"]).ToNot(BeNil())
			Expect(dnsSet.Sets["A"].TTL).To(Equal(int64(300)))
			Expect(dnsSet.Sets["A"].Records).To(HaveLen(2))
			Expect(dnsSet.Sets["A"].Records[0].Value).To(Equal("1.2.3.4"))
			Expect(dnsSet.Sets["A"].Records[1].Value).To(Equal("5.6.7.8"))
			Expect(dnsSet.Sets["AAAA"]).ToNot(BeNil())
			Expect(dnsSet.Sets["AAAA"].TTL).To(Equal(int64(100)))
			Expect(dnsSet.Sets["AAAA"].Records).To(HaveLen(1))
			Expect(dnsSet.Sets["AAAA"].Records[0].Value).To(Equal("::1"))
		})
	})

	Describe("Match", func() {
		It("should match DNSSet correctly", func() {
			dnsSet1 := &DNSSet{
				Name:        DNSSetName{DNSName: "example.com"},
				UpdateGroup: "group1",
				Sets: RecordSets{
					"A": &RecordSet{Type: "A", TTL: 300, Records: []*Record{{Value: "1.2.3.4"}}},
				},
				RoutingPolicy: &RoutingPolicy{Type: RoutingPolicyWeighted},
			}
			dnsSet2 := dnsSet1.Clone()
			Expect(dnsSet1.Match(dnsSet2)).To(BeTrue())
		})
	})

	Describe("MatchRecordTypeSubset", func() {
		It("should match DNSSet record type subset correctly", func() {
			dnsSet1 := &DNSSet{
				Name:        DNSSetName{DNSName: "example.com"},
				UpdateGroup: "group1",
				Sets: RecordSets{
					"A":   &RecordSet{Type: "A", TTL: 300, Records: []*Record{{Value: "1.2.3.4"}}},
					"TXT": &RecordSet{Type: "TXT", TTL: 300, Records: []*Record{{Value: "v=spf1 include:_spf.example.com ~all"}}},
				},
				RoutingPolicy: &RoutingPolicy{Type: RoutingPolicyWeighted},
			}
			dnsSet2 := dnsSet1.Clone()
			Expect(dnsSet1.MatchRecordTypeSubset(dnsSet2, "A")).To(BeTrue())
			Expect(dnsSet1.MatchRecordTypeSubset(dnsSet2, "TXT")).To(BeTrue())
		})
	})
})

var _ = Describe("DNSSets", func() {
	Describe("AddRecordSet", func() {
		It("should add a record set correctly", func() {
			dnsSets := DNSSets{}
			name := DNSSetName{DNSName: "example.com"}
			policy := &RoutingPolicy{Type: RoutingPolicyWeighted}
			recordSet := &RecordSet{Type: "A", TTL: 300, Records: []*Record{{Value: "1.2.3.4"}}}
			recordSetUpdate := &RecordSet{Type: "A", TTL: 301, Records: []*Record{{Value: "1.2.3.4"}, {"5.6.7.8"}}}

			dnsSets.AddRecordSet(name, policy, recordSet)
			dnsSets.AddRecordSet(name, policy, recordSetUpdate)

			name2 := DNSSetName{DNSName: "www.example.com"}
			dnsSets.AddRecordSet(name2, nil, recordSet)

			Expect(dnsSets[name]).ToNot(BeNil())
			Expect(dnsSets[name].Sets["A"]).To(Equal(recordSetUpdate))
			Expect(dnsSets[name].RoutingPolicy).To(Equal(policy))

			Expect(dnsSets[name2]).ToNot(BeNil())
			Expect(dnsSets[name2].Sets["A"]).To(Equal(recordSet))
			Expect(dnsSets[name2].RoutingPolicy).To(BeNil())
		})
	})

	Describe("RemoveRecordSet", func() {
		It("should remove a record set correctly", func() {
			dnsSets := DNSSets{}
			name := DNSSetName{DNSName: "example.com"}
			recordSet := &RecordSet{Type: "A", TTL: 300, Records: []*Record{{Value: "1.2.3.4"}}}

			dnsSets.AddRecordSet(name, &RoutingPolicy{Type: RoutingPolicyWeighted}, recordSet)
			dnsSets.RemoveRecordSet(name, "A")

			Expect(dnsSets[name]).To(BeNil())
		})
	})

	Describe("Clone", func() {
		It("should clone DNSSets correctly", func() {
			dnsSets := DNSSets{}
			name := DNSSetName{DNSName: "example.com"}
			recordSet := &RecordSet{Type: "A", TTL: 300, Records: []*Record{{Value: "1.2.3.4"}}}

			dnsSets.AddRecordSet(name, &RoutingPolicy{Type: RoutingPolicyWeighted}, recordSet)
			clone := dnsSets.Clone()

			Expect(clone).To(Equal(dnsSets))
			Expect(clone).ToNot(BeIdenticalTo(dnsSets))
		})
	})
})
