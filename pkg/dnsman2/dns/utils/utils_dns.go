// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import "strings"

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
