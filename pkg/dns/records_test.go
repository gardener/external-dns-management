package dns

import (
	"encoding/json"
	"testing"
)

func TestMatch(t *testing.T) {
	/* Testing for :
	- Equality of two record sets is determined based on TTL, length of record entries amd equality of the record values
	- ProviderType of the record set (A Record, TXT) should not make a difference
	*/

	table := []struct {
		recordSetOne       RecordSet
		recordSetTwo       RecordSet
		recordSetsAreEqual bool
	}{
		// Equal Sets
		{RecordSet{Type: RS_TXT, TTL: 600, Records: []*Record{{"\"foo\""}}}, RecordSet{Type: RS_TXT, TTL: 600, Records: []*Record{{"\"foo\""}}}, true},
		// One record value different = not equal
		{RecordSet{Type: RS_TXT, TTL: 600, Records: []*Record{{"\"foo\""}}}, RecordSet{Type: RS_TXT, TTL: 600, Records: []*Record{{"xx.xx.xx.xx"}}}, false},
		// Equal except for TTL = not equal
		{RecordSet{Type: RS_TXT, TTL: 600, Records: []*Record{{"\"foo\""}}}, RecordSet{Type: RS_TXT, TTL: 800, Records: []*Record{{"\"foo\""}}}, false},
		// different amount of records = not equal
		{RecordSet{Type: RS_TXT, TTL: 600, Records: []*Record{{"\"foo\""}}}, RecordSet{Type: RS_TXT, TTL: 600, Records: []*Record{{"\"foo\""}, {"\"foo\""}}}, false},
		// different type = not equal
		{RecordSet{Type: RS_A, TTL: 600, Records: []*Record{{"1.2.3.4"}}}, RecordSet{Type: RS_TXT, TTL: 600, Records: []*Record{{"\"foo\""}}}, false},
		// alias type: ignore TTL as ignored in AWS Route53 anyway
		{RecordSet{Type: RS_ALIAS_A, TTL: 60, Records: []*Record{{"foo.example.com"}}}, RecordSet{Type: RS_ALIAS_A, TTL: 0, Records: []*Record{{"foo.example.com"}}}, true},
		// alias_a type: ignore TTL as ignored in AWS Route53 anyway
		{RecordSet{Type: RS_ALIAS_AAAA, TTL: 60, Records: []*Record{{"foo.example.com"}}}, RecordSet{Type: RS_ALIAS_AAAA, TTL: 0, Records: []*Record{{"foo.example.com"}}}, true},
	}

	for _, entry := range table {
		isEqual := entry.recordSetOne.Match(&entry.recordSetTwo)

		if isEqual != entry.recordSetsAreEqual {
			one, _ := json.MarshalIndent(entry.recordSetOne, "", "  ")
			two, _ := json.MarshalIndent(entry.recordSetTwo, "", "  ")
			t.Errorf("Wrong result. RecordSets are equal: %t.  RecordSetOne: '%v' and RecordsSetTwo '%v'", entry.recordSetsAreEqual, string(one), string(two))
		}
	}
}
