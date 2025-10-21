/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package testutils

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// AssertEvents checks for expected events (in the given order).
func AssertEvents(actual <-chan string, expectedList ...string) {
	c := time.After(1 * time.Second)
	for _, e := range expectedList {
		select {
		case a := <-actual:
			if !strings.HasPrefix(a, e) {
				ExpectWithOffset(1, a).To(ContainSubstring(e))
				return
			}
		case <-c:
			Fail(fmt.Sprintf("Expected event %q, got nothing", e))
			// continue iterating to print all expected events
		}
	}
	for {
		select {
		case a := <-actual:
			Fail(fmt.Sprintf("Unexpected event: %q", a))
		default:
			return // No more events, as expected.
		}
	}
}

// AssertUnorderedEvents checks for expected events (any order).
func AssertUnorderedEvents(actual <-chan string, expectedList ...string) {
	c := time.After(1 * time.Second)
	var actualList []string
	for _, e := range expectedList {
		select {
		case a := <-actual:
			actualList = append(actualList, a)
		case <-c:
			Fail(fmt.Sprintf("Expected event %q, got nothing", e))
			// continue iterating to print all expected events
		}
	}
outer:
	for {
		select {
		case a := <-actual:
			actualList = append(actualList, a)
		default:
			break outer // No more events, as expected.
		}
	}

	missing := []string{}
outer2:
	for _, e := range expectedList {
		for i, a := range actualList {
			if strings.HasPrefix(a, e) {
				actualList = append(actualList[:i], actualList[i+1:]...)
				continue outer2
			}
		}
		missing = append(missing, e)
	}
	// show mismatches
	Expect(missing).To(Equal(actualList))
}
