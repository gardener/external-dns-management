package dns

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RecordSet", func() {
	var (
		rs1 = RecordSet{
			Type:    TypeTXT,
			TTL:     600,
			Records: []*Record{{Value: "\"foo\""}},
		}
		makeAliasRecordSet = func(recordType RecordType, ttl int64) RecordSet {
			return RecordSet{
				Type:    recordType,
				TTL:     ttl,
				Records: []*Record{{Value: "foo.example.com"}},
			}
		}
		makeRecordSetWithRoutingPolicy = func(routingPolicy *RoutingPolicy) RecordSet {
			rs := rs1.Clone()
			rs.RoutingPolicy = routingPolicy
			return *rs
		}
		policy1 = &RoutingPolicy{
			Type:       RoutingPolicyWeighted,
			Parameters: map[string]string{"weight": "100"},
		}
		policy2 = &RoutingPolicy{
			Type:       RoutingPolicyWeighted,
			Parameters: map[string]string{"weight": "50"},
		}
		rs1p1 = makeRecordSetWithRoutingPolicy(policy1)
	)

	DescribeTable("Match",
		func(recordSetOne, recordSetTwo RecordSet, recordSetsAreEqual bool) {
			isEqual := recordSetOne.Match(&recordSetTwo)
			Expect(isEqual).To(Equal(recordSetsAreEqual))
			isEqual = recordSetTwo.Match(&recordSetOne)
			Expect(isEqual).To(Equal(recordSetsAreEqual))
		},
		Entry("Equal Sets", rs1, rs1, true),
		Entry("Equal Sets (cloned)", rs1, *rs1.Clone(), true),
		Entry("One record value different", rs1, RecordSet{Type: TypeTXT, TTL: 600, Records: []*Record{{Value: "xx.xx.xx.xx"}}}, false),
		Entry("Equal except for TTL", rs1, RecordSet{Type: TypeTXT, TTL: 800, Records: []*Record{{Value: "\"foo\""}}}, false),
		Entry("Different amount of records", rs1, RecordSet{Type: TypeTXT, TTL: 600, Records: []*Record{{Value: "\"foo\""}, {Value: "\"foo\""}}}, false),
		Entry("Different type", rs1, RecordSet{Type: TypeA, TTL: 600, Records: []*Record{{Value: "1.2.3.4"}}}, false),
		Entry("Alias type: ignore TTL", makeAliasRecordSet(TypeAWS_ALIAS_A, 60), makeAliasRecordSet(TypeAWS_ALIAS_A, 0), true),
		Entry("Alias_AAAA type: ignore TTL", makeAliasRecordSet(TypeAWS_ALIAS_AAAA, 60), makeAliasRecordSet(TypeAWS_ALIAS_AAAA, 0), true),
		Entry("Equal Sets with routing policy", rs1p1, rs1p1, true),
		Entry("Equal Sets with routing policy (cloned)", rs1p1, *rs1p1.Clone(), true),
		Entry("Switch off routing policy", rs1p1, rs1, false),
		Entry("Different routing policy", rs1p1, makeRecordSetWithRoutingPolicy(policy2), false),
	)
})
