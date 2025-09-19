// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cloudflare

import (
	cloudflaredns "github.com/cloudflare/cloudflare-go/v6/dns"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Record cloudflaredns.RecordResponse

var _ raw.Record = &Record{}

func (r *Record) GetType() string          { return string(r.Type) }
func (r *Record) GetId() string            { return r.ID }
func (r *Record) GetDNSName() string       { return r.Name }
func (r *Record) GetSetIdentifier() string { return "" }
func (r *Record) GetValue() string {
	if r.Type == dns.RS_TXT {
		return raw.EnsureQuotedText(r.Content)
	}
	return r.Content
}
func (r *Record) GetTTL() int64    { return int64(r.TTL) }
func (r *Record) SetTTL(ttl int64) { r.TTL = cloudflaredns.TTL(ttl) }
func (r *Record) Copy() raw.Record { n := *r; return &n }

func (r *Record) SetRoutingPolicy(string, *dns.RoutingPolicy) {}
