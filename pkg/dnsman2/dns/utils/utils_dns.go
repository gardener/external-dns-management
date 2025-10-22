// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"strings"
)

// TTLToUint32 converts a TTL value to an uint32 value.
func TTLToUint32(ttl int64) uint32 {
	if ttl < 0 {
		return 0
	}
	if ttl > 0xFFFFFFFF {
		return 0xFFFFFFFF
	}
	return uint32(ttl)
}

// TTLToInt32 converts a TTL value to an int32 value.
func TTLToInt32(ttl int64) int32 {
	if ttl < 0 {
		return 0
	}
	if ttl > 0x7FFFFFFF {
		return 0x7FFFFFFF
	}
	return int32(ttl)
}

// Match returns true if the hostname is the domain or a subdomain.
func Match(hostname, domain string) bool {
	return strings.HasSuffix(hostname, "."+domain) || domain == hostname
}

// UniqueStrings is a list of unique strings.
type UniqueStrings []string

// NewUniqueStrings creates a new UniqueStrings instance.
func NewUniqueStrings() *UniqueStrings {
	var u UniqueStrings
	return &u
}

// Add adds the string to the unique strings if not already present.
func (u *UniqueStrings) Add(s string) {
	for _, v := range *u {
		if v == s {
			return
		}
	}
	*u = append(*u, s)
}

// AddAll adds all strings from the slice to the unique strings.
func (u *UniqueStrings) AddAll(ss []string) {
	for _, s := range ss {
		u.Add(s)
	}
}

// Remove removes the string from the unique strings.
func (u *UniqueStrings) Remove(s string) {
	var newList []string
	for _, v := range *u {
		if v != s {
			newList = append(newList, v)
		}
	}
	*u = newList
}

// Contains returns true if the string is in the unique strings.
func (u *UniqueStrings) Contains(s string) bool {
	for _, v := range *u {
		if v == s {
			return true
		}
	}
	return false
}

// Len returns the number of unique strings.
func (u *UniqueStrings) Len() int {
	return len(*u)
}

// IsEmpty returns true if there are no unique strings.
func (u *UniqueStrings) IsEmpty() bool {
	return u == nil || len(*u) == 0
}

// ToSlice returns a copy of the unique strings as a slice.
func (u *UniqueStrings) ToSlice() []string {
	if u == nil || len(*u) == 0 {
		return nil
	}
	clone := make([]string, len(*u))
	copy(clone, *u)
	return clone
}
