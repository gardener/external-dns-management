package dns

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RecordSet", func() {
	DescribeTable("Match",
		func(recordSetOne, recordSetTwo RecordSet, recordSetsAreEqual bool) {
			isEqual := recordSetOne.Match(&recordSetTwo)
			Expect(isEqual).To(Equal(recordSetsAreEqual))
		},
		Entry("Equal Sets", RecordSet{Type: TypeTXT, TTL: 600, Records: []*Record{{Value: "\"foo\""}}}, RecordSet{Type: TypeTXT, TTL: 600, Records: []*Record{{Value: "\"foo\""}}}, true),
		Entry("One record value different", RecordSet{Type: TypeTXT, TTL: 600, Records: []*Record{{Value: "\"foo\""}}}, RecordSet{Type: TypeTXT, TTL: 600, Records: []*Record{{Value: "xx.xx.xx.xx"}}}, false),
		Entry("Equal except for TTL", RecordSet{Type: TypeTXT, TTL: 600, Records: []*Record{{Value: "\"foo\""}}}, RecordSet{Type: TypeTXT, TTL: 800, Records: []*Record{{Value: "\"foo\""}}}, false),
		Entry("Different amount of records", RecordSet{Type: TypeTXT, TTL: 600, Records: []*Record{{Value: "\"foo\""}}}, RecordSet{Type: TypeTXT, TTL: 600, Records: []*Record{{Value: "\"foo\""}, {Value: "\"foo\""}}}, false),
		Entry("Different type", RecordSet{Type: TypeA, TTL: 600, Records: []*Record{{Value: "1.2.3.4"}}}, RecordSet{Type: TypeTXT, TTL: 600, Records: []*Record{{Value: "\"foo\""}}}, false),
		Entry("Alias type: ignore TTL", RecordSet{Type: TypeAWS_ALIAS_A, TTL: 60, Records: []*Record{{Value: "foo.example.com"}}}, RecordSet{Type: TypeAWS_ALIAS_A, TTL: 0, Records: []*Record{{Value: "foo.example.com"}}}, true),
		Entry("Alias_AAAA type: ignore TTL", RecordSet{Type: TypeAWS_ALIAS_AAAA, TTL: 60, Records: []*Record{{Value: "foo.example.com"}}}, RecordSet{Type: TypeAWS_ALIAS_AAAA, TTL: 0, Records: []*Record{{Value: "foo.example.com"}}}, true),
	)
})
