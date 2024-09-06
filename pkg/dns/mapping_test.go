// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
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
