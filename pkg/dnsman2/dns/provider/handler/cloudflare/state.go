// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cloudflare

import (
	cloudflaredns "github.com/cloudflare/cloudflare-go/v6/dns"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/raw"
)

// SetIdentifierProxied is the set identifier used for proxied records in Cloudflare.
const SetIdentifierProxied = "proxied"

// Record represents a DNS record in Cloudflare.
type Record cloudflaredns.RecordResponse

var _ raw.Record = &Record{}

// GetType returns the record type.
func (r *Record) GetType() string { return string(r.Type) }

// GetId returns the record ID.
func (r *Record) GetId() string { return r.ID }

// GetDNSName returns the DNS name of the record.
func (r *Record) GetDNSName() string { return r.Name }

// GetSetIdentifier returns the set identifier, returning "proxied" if the record is proxied, otherwise an empty string.
func (r *Record) GetSetIdentifier() string {
	if r.Proxied {
		return SetIdentifierProxied
	}
	return ""
}

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

// SetRoutingPolicy is used to set the routing policy of the record based on the set identifier.
// Only if the set identifier is "proxied" and the policy type is "proxied", the record will be marked as proxied.
func (r *Record) SetRoutingPolicy(setIdentifier string, policy *dns.RoutingPolicy) {
	r.Proxied = setIdentifier == SetIdentifierProxied && policy != nil && policy.Type == dns.RoutingPolicyProxied
}
