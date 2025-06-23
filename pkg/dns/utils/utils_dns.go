// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"github.com/gardener/external-dns-management/pkg/dns"
)

type TargetProvider interface {
	Targets() Targets
	TTL() int64
	RoutingPolicy() *dns.RoutingPolicy
}

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
