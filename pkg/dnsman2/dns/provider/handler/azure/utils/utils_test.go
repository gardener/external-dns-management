// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package utils

import "testing"

func TestDropZoneName(t *testing.T) {
	for _, tc := range []struct {
		dnsName  string
		zoneName string
		want     string
		wantOK   bool
	}{
		{"www2.test.ml", "test.ml", "www2", true},
		{"a.b.test.ml", "test.ml", "a.b", true},
		{"test.ml", "test.ml", "test.ml", false}, // apex: not zone-relative, no substitution on query
		{"other.ml", "test.ml", "other.ml", false},
	} {
		got, ok := DropZoneName(tc.dnsName, tc.zoneName)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("DropZoneName(%q, %q) = (%q, %v), want (%q, %v)", tc.dnsName, tc.zoneName, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestRelativeRecordSetName(t *testing.T) {
	for _, tc := range []struct {
		dnsName  string
		zoneName string
		want     string
		wantOK   bool
	}{
		{"www2.test.ml", "test.ml", "www2", true},
		{"test.ml", "test.ml", "@", true}, // apex maps to "@" for writing
		{"other.ml", "test.ml", "other.ml", false},
	} {
		got, ok := RelativeRecordSetName(tc.dnsName, tc.zoneName)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("RelativeRecordSetName(%q, %q) = (%q, %v), want (%q, %v)", tc.dnsName, tc.zoneName, got, ok, tc.want, tc.wantOK)
		}
	}
}
