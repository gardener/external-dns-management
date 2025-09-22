// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cloudflare

import (
	cloudflaredns "github.com/cloudflare/cloudflare-go/v6/dns"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/raw"
)

// Record represents a DNS record in Cloudflare.
type Record cloudflaredns.RecordResponse

var _ raw.Record = &Record{}

// GetType returns the record type.
func (r *Record) GetType() string { return string(r.Type) }

// GetId returns the record ID.
func (r *Record) GetId() string { return r.ID }

// GetDNSName returns the DNS name of the record.
func (r *Record) GetDNSName() string { return r.Name }

// GetSetIdentifier returns the set identifier, which is not used in Cloudflare.
func (r *Record) GetSetIdentifier() string { return "" }

// GetValue returns the content of the record, ensuring TXT records are quoted.
func (r *Record) GetValue() string {
	if r.Type == cloudflaredns.RecordResponseTypeTXT {
		return raw.EnsureQuotedText(r.Content)
	}
	return r.Content
}

// GetTTL returns the TTL of the record.
func (r *Record) GetTTL() int64 { return int64(r.TTL) }

// SetTTL sets the TTL of the record.
func (r *Record) SetTTL(ttl int64) { r.TTL = cloudflaredns.TTL(ttl) }

// Clone creates a copy of the record.
func (r *Record) Clone() raw.Record { n := *r; return &n }

// SetRoutingPolicy is a no-op for Cloudflare as it does not support routing policies.
func (r *Record) SetRoutingPolicy(string, *dns.RoutingPolicy) {}
