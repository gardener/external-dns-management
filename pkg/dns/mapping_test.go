/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

 package dns

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAlignHostname(t *testing.T) {
	if AlignHostname("a.b") != "a.b." {
		t.Error("Expected 'a.b' changed to 'a.b.'")
	}
	if AlignHostname("a.b.") != "a.b." {
		t.Error("Expected 'a.b.' unchanged")
	}
}

func TestNormalizeHostname(t *testing.T) {
	table := []struct {
		input  string
		wanted string
	}{
		{"a.b", "a.b"},
		{"a.b.", "a.b"},
		{"*.a", "*.a"},
		{"\\052.a.b", "*.a.b"},
	}
	for _, entry := range table {
		result := NormalizeHostname(entry.input)
		if result != entry.wanted {
			t.Errorf("%s: wanted %s, but got %s", entry.input, entry.wanted, result)
		}
	}
}

func TestMapToFromProvider(t *testing.T) {
	table := []struct {
		domainName          string
		hasOwnCommentRecord bool
		wantedName          string
	}{
		{"a.myzone.de", false, "comment-a.myzone.de"},
		{"a.myzone.de", true, "mycomment-a.myzone.de"},
		{"*.a.myzone.de", false, "*.comment-a.myzone.de"},
		{"*.myzone.de", false, "*.comment-.myzone.de"},
	}

	rtype := RS_META
	base := "myzone.de"

	for _, entry := range table {
		inputRecords := []*Record{&Record{"\"owner=test\""}}
		var wantedRecords []*Record
		if entry.hasOwnCommentRecord {
			inputRecords = append(inputRecords, &Record{"\"prefix=mycomment-\""})
			wantedRecords = inputRecords
		} else {
			wantedRecords = append(inputRecords, &Record{"\"prefix=comment-\""})
		}
		dnsset := DNSSet{
			Name: entry.domainName,
			Sets: RecordSets{RS_META: &RecordSet{Type: RS_META, TTL: 600, Records: inputRecords}},
		}

		actualName, actualRecordSet := MapToProvider(rtype, &dnsset, base)

		if actualName != entry.wantedName {
			t.Errorf("Name mismatch: %s != %s", entry.wantedName, actualName)
		}
		if actualRecordSet.Type != RS_TXT {
			t.Errorf("RecordSet.Type mismatch: %v != TXT", actualRecordSet.Type)
		}
		if actualRecordSet.TTL != 600 {
			t.Errorf("TTL mismatch: %v != 600", actualRecordSet.TTL)
		}
		if !reflect.DeepEqual(wantedRecords, actualRecordSet.Records) {
			w, _ := json.MarshalIndent(wantedRecords, "", "  ")
			a, _ := json.MarshalIndent(actualRecordSet.Records, "", "  ")
			t.Errorf("Record set mismatch: %v != %v", string(w), string(a))
		}

		reversedName, reversedRecordSet := MapFromProvider(actualName, actualRecordSet)

		if reversedName != entry.domainName {
			t.Errorf("Reversed name mismatch: %s != %s", reversedName, entry.domainName)
		}
		if reversedRecordSet.Type != RS_META {
			t.Errorf("Reversed RecordSet.Type mismatch: %v != RS_META", reversedRecordSet.Type)
		}
		if reversedRecordSet.TTL != 600 {
			t.Errorf("Reversed TTL mismatch: %v != 600", reversedRecordSet.TTL)
		}
		if !reflect.DeepEqual(reversedRecordSet.Records, wantedRecords) {
			w, _ := json.MarshalIndent(reversedRecordSet.Records, "", "  ")
			a, _ := json.MarshalIndent(wantedRecords, "", "  ")
			t.Errorf("Reversed Record set mismatch: %v != %v", string(w), string(a))
		}
	}
}
