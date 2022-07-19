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
	"testing"

	. "github.com/onsi/gomega"
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
	RegisterTestingT(t)

	table := []struct {
		domainName          string
		hasOwnCommentRecord bool
		wantedName          string
	}{
		{"a.myzone.de", false, "comment-a.myzone.de"},
		{"a.myzone.de", true, "mycomment-a.myzone.de"},
		{"*.a.myzone.de", false, "*.comment-a.myzone.de"},
		{"*.myzone.de", false, "*.comment--base.myzone.de"},
	}

	rtype := RS_META
	base := "myzone.de"

	for _, entry := range table {
		inputRecords := Records{&Record{"\"owner=test\""}}
		var wantedRecords Records
		if entry.hasOwnCommentRecord {
			inputRecords = append(inputRecords, &Record{"\"prefix=mycomment-\""})
			wantedRecords = inputRecords
		} else {
			wantedRecords = append(inputRecords, &Record{"\"prefix=comment-\""})
		}
		dnsset := DNSSet{
			Name: DNSSetName{DNSName: entry.domainName},
			Sets: RecordSets{RS_META: &RecordSet{Type: RS_META, TTL: 600, Records: inputRecords}},
		}

		actualName, actualRecordSet := MapToProvider(rtype, &dnsset, base)

		Ω(actualName).Should(Equal(DNSSetName{DNSName: entry.wantedName}), "Name should match")
		Ω(actualRecordSet.Type).Should(Equal(RS_TXT), "Type mismatch")
		Ω(actualRecordSet.TTL).Should(Equal(int64(600)), "TTL mismatch")
		Ω(actualRecordSet.Records).Should(Equal(wantedRecords))

		reversedName, reversedRecordSet := MapFromProvider(actualName, actualRecordSet)

		Ω(reversedName).Should(Equal(DNSSetName{DNSName: entry.domainName}), "Reversed name should match")
		Ω(reversedRecordSet.Type).Should(Equal(RS_META), "Reversed RecordSet.Type should match")
		Ω(reversedRecordSet.TTL).Should(Equal(int64(600)), "TTL mismatch")
		Ω(reversedRecordSet.Records).Should(Equal(wantedRecords))
	}
}
