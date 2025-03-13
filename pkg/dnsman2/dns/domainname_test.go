// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns_test

import (
	"testing"

	. "github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

func TestEnsureTrailingDot(t *testing.T) {
	if EnsureTrailingDot("a.b") != "a.b." {
		t.Error("Expected 'a.b' changed to 'a.b.'")
	}
	if EnsureTrailingDot("a.b.") != "a.b." {
		t.Error("Expected 'a.b.' unchanged")
	}
}

func TestNormalizeDomainName(t *testing.T) {
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
		result := NormalizeDomainName(entry.input)
		if result != entry.wanted {
			t.Errorf("%s: wanted %s, but got %s", entry.input, entry.wanted, result)
		}
	}
}
