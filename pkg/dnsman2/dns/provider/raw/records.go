// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package raw

import (
	"strconv"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// Record is the interface for supporting handling with raw DNS records in a DNS provider.
type Record interface {
	// GetId returns the provider specific unique identifier of the record.
	// Note: This ID is not the same as the DNS name of the record.
	GetId() string
	// GetType returns the DNS record type (e.g. A, CNAME, TXT, etc.).
	GetType() string
	// GetValue returns the value of the DNS record.
	GetValue() string
	// GetDNSName returns the fully qualified domain name of the DNS record.
	GetDNSName() string
	// GetSetIdentifier returns the set identifier of the DNS record (if any).
	// If the record does not have a set identifier, an empty string is returned.
	GetSetIdentifier() string
	// GetTTL returns the TTL of the DNS record.
	GetTTL() int64
	// SetTTL sets the TTL of the DNS record.
	SetTTL(int64)
	// SetRoutingPolicy sets the routing policy of the DNS record.
	SetRoutingPolicy(setIdentifier string, policy *dns.RoutingPolicy)
	// Clone returns a deep copy of the DNS record.
	Clone() Record
}

// RecordList is a list of Records.
type RecordList []Record

// Clone returns a deep copy of the RecordList.
func (this RecordList) Clone() RecordList {
	clone := make(RecordList, len(this))
	for i, r := range this {
		clone[i] = r.Clone()
	}
	return clone
}

// EnsureQuotedText ensures that the given string is properly quoted for TXT records.
func EnsureQuotedText(v string) string {
	if _, err := strconv.Unquote(v); err != nil {
		v = strconv.Quote(v)
	}
	return v
}
