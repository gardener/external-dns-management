// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import "testing"

func TestDropZoneName(t *testing.T) {
	table := []struct {
		dnsName      string
		zoneName     string
		expectedName string
		ok           bool
	}{
		{"www.test.com", "test.com", "www", true},
		{"w.test.com", "test.com", "w", true},
		{"test.com", "test.com", "", false},
		{".test.com", "test.com", "", false},
		{"w.test.COM", "test.com", "", false},
	}
	for _, entry := range table {
		name, ok := DropZoneName(entry.dnsName, entry.zoneName)
		if ok != entry.ok {
			t.Errorf("Failed: unexpected ok: %v!=%v for %v", ok, entry.ok, entry)
		}
		if ok && name != entry.expectedName {
			t.Errorf("Failed: unexpected name: %s!=%s for %v", name, entry.expectedName, entry)
		}
	}
}
