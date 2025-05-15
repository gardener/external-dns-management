// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package netlify

import (
	"github.com/netlify/open-api/go/models"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Record models.DNSRecord

var _ raw.Record = &Record{}

func (r *Record) GetType() string          { return r.Type }
func (r *Record) GetId() string            { return r.ID }
func (r *Record) GetDNSName() string       { return r.Hostname }
func (r *Record) GetSetIdentifier() string { return "" }
func (r *Record) GetValue() string {
	if r.Type == dns.RS_TXT {
		return raw.EnsureQuotedText(r.Value)
	}
	return r.Value
}
func (r *Record) GetTTL() int64    { return r.TTL }
func (r *Record) SetTTL(ttl int64) { r.TTL = ttl }
func (r *Record) Copy() raw.Record { n := *r; return &n }

func (r *Record) SetRoutingPolicy(setIdentifier string, policy *dns.RoutingPolicy) {}
