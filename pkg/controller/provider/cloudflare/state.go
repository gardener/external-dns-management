// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cloudflare

import (
	"github.com/cloudflare/cloudflare-go"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Record cloudflare.DNSRecord

var _ raw.Record = &Record{}

func (r *Record) GetType() string          { return r.Type }
func (r *Record) GetId() string            { return r.ID }
func (r *Record) GetDNSName() string       { return r.Name }
func (r *Record) GetSetIdentifier() string { return "" }
func (r *Record) GetValue() string {
	if r.Type == dns.RS_TXT {
		return raw.EnsureQuotedText(r.Content)
	}
	return r.Content
}
func (r *Record) GetTTL() int      { return r.TTL }
func (r *Record) SetTTL(ttl int)   { r.TTL = ttl }
func (r *Record) Copy() raw.Record { n := *r; return &n }
