// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"fmt"
	"testing"
)

const name23 = "ab1234567890-1234567890"

func TestValidation(t *testing.T) {
	debug := false

	name239 := name23
	for i := 0; i < 9; i++ {
		name239 += "." + name23
	}
	table := []struct {
		input string
		ok    bool
	}{
		{"a.b", true},
		{"a.b.", true},
		{"*.a", true},
		{"\\052.a.b", true},
		{"a-a.a9.a8.a7.a6.a5.a4.a3.a2.a1.a.b.c.d.e.f.g.h.i.j.k.l.m.n.o.p.q.r.s.t.u.v.w.x.y.z", true},
		{"_a.b", true},
		{"1.2-3.b", true},
		{"a123456789012345678901234567890123456789012345678901234567890abc.b", false},   // label too long
		{"a.a123456789012345678901234567890123456789012345678901234567890abc.b", false}, // label too long
		{"a12345678901234567890123456789012345678901234567890abcd.b", true},
		{"a12345678901234567890123456789012345678901234567890abcde.b", false}, // meta data label too long
		{"abc.a123456789." + name239, false},                                  // name too long
		{"abcde." + name239, true},                                            // comment-abcde... has 253 chars
		{"abcdef." + name239, false},                                          // meta data name too long
	}
	for _, entry := range table {
		err := ValidateDomainName(entry.input)
		if debug && err != nil {
			fmt.Printf("%v\n", err)
		}
		if entry.ok && err != nil {
			t.Errorf("%s should be ok, but got error %s", entry.input, err)
		} else if !entry.ok && err == nil {
			t.Errorf("%s should not be ok, but got no error", entry.input)
		}
	}
}
