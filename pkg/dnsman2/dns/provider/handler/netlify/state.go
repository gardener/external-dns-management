// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package netlify

import (
	"github.com/netlify/open-api/go/models"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/raw"
)

// Record is an implementation of raw.Record for Netlify DNS records
type Record models.DNSRecord

var _ raw.Record = &Record{}

// GetType returns the record type.
func (r *Record) GetType() string { return r.Type }

// GetId returns the record ID.
func (r *Record) GetId() string { return r.ID }

// GetDNSName returns the DNS name of the record.
func (r *Record) GetDNSName() string { return r.Hostname }

// GetSetIdentifier returns the set identifier of the record, which is not used in Netlify DNS.
func (r *Record) GetSetIdentifier() string { return "" }

// GetValue returns the value of the record. For TXT records, it ensures the text is quoted.
func (r *Record) GetValue() string {
	if r.Type == string(dns.TypeTXT) {
		return raw.EnsureQuotedText(r.Value)
	}
	return r.Value
}

// GetTTL returns the TTL of the record.
func (r *Record) GetTTL() int64 { return r.TTL }

// SetTTL sets the TTL of the record.
func (r *Record) SetTTL(ttl int64) { r.TTL = ttl }

// Clone creates a deep copy of the record.
func (r *Record) Clone() raw.Record { n := *r; return &n }

// SetRoutingPolicy sets the routing policy of the record, which is not supported in Netlify DNS.
func (r *Record) SetRoutingPolicy(string, *dns.RoutingPolicy) {}
