package dns

import (
	"encoding/json"
	"testing"
)

func TestMatch(t *testing.T) {
	/* Testing for :
	- Equality of two record sets is determined based on TTL, length of record entries amd equality of the record values
	- Type of the record set (A Record, TXT) should not make a difference
	*/

	table := []struct {
		recordSetOne       RecordSet
		recordSetTwo       RecordSet
		recordSetsAreEqual bool
	}{
		// Equal Sets
		{RecordSet{Type: RS_META, TTL: 600, Records: []*Record{&Record{"\"owner=test\""}}}, RecordSet{Type: RS_META, TTL: 600, Records: []*Record{&Record{"\"owner=test\""}}}, true},
		// RecordSet type not equal TTL & records equal = equal
		{RecordSet{Type: RS_META, TTL: 600, Records: []*Record{&Record{"\"owner=test\""}}}, RecordSet{Type: RS_TXT, TTL: 600, Records: []*Record{&Record{"\"owner=test\""}}}, true},
		//One record value different = not equal
		{RecordSet{Type: RS_META, TTL: 600, Records: []*Record{&Record{"\"owner=test\""}}}, RecordSet{Type: RS_META, TTL: 600, Records: []*Record{&Record{"xx.xx.xx.xx"}}}, false},
		// Equal except for TTL = not equal
		{RecordSet{Type: RS_META, TTL: 600, Records: []*Record{&Record{"\"owner=test\""}}}, RecordSet{Type: RS_TXT, TTL: 800, Records: []*Record{&Record{"\"owner=test\""}}}, false},
		// different amount of records = not equal
		{RecordSet{Type: RS_META, TTL: 600, Records: []*Record{&Record{"\"owner=test\""}}}, RecordSet{Type: RS_TXT, TTL: 600, Records: []*Record{&Record{"\"owner=test\""}, &Record{"\"owner=test\""}}}, false},
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
