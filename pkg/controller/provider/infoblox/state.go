// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package infoblox

import (
	ibclient "github.com/infobloxopen/infoblox-go-client/v2"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
	"github.com/gardener/external-dns-management/pkg/dns/utils"
)

type Record interface {
	raw.Record
	PrepareUpdate() raw.Record
}

type RecordA ibclient.RecordA

func (r *RecordA) GetType() string          { return dns.RS_A }
func (r *RecordA) GetId() string            { return r.Ref }
func (r *RecordA) GetDNSName() string       { return r.Name }
func (r *RecordA) GetSetIdentifier() string { return "" }
func (r *RecordA) GetValue() string         { return r.Ipv4Addr }
func (r *RecordA) GetTTL() int64            { return int64(r.Ttl) }
func (r *RecordA) SetTTL(ttl int64)         { r.Ttl = utils.TTLToUint32(ttl); r.UseTtl = ttl != 0 }
func (r *RecordA) Copy() raw.Record         { n := *r; return &n }
func (r *RecordA) PrepareUpdate() raw.Record {
	n := *r
	n.Zone = ""
	n.Name = ""
	n.View = ""
	return &n
}
func (r *RecordA) SetRoutingPolicy(setIdentifier string, policy *dns.RoutingPolicy) {}

type RecordAAAA ibclient.RecordAAAA

func (r *RecordAAAA) GetType() string          { return dns.RS_AAAA }
func (r *RecordAAAA) GetId() string            { return r.Ref }
func (r *RecordAAAA) GetDNSName() string       { return r.Name }
func (r *RecordAAAA) GetSetIdentifier() string { return "" }
func (r *RecordAAAA) GetValue() string         { return r.Ipv6Addr }
func (r *RecordAAAA) GetTTL() int64            { return int64(r.Ttl) }
func (r *RecordAAAA) SetTTL(ttl int64)         { r.Ttl = utils.TTLToUint32(ttl); r.UseTtl = ttl != 0 }
func (r *RecordAAAA) Copy() raw.Record         { n := *r; return &n }
func (r *RecordAAAA) PrepareUpdate() raw.Record {
	n := *r
	n.Zone = ""
	n.Name = ""
	n.View = ""
	return &n
}

func (r *RecordAAAA) SetRoutingPolicy(setIdentifier string, policy *dns.RoutingPolicy) {}

type RecordCNAME ibclient.RecordCNAME

func (r *RecordCNAME) GetType() string           { return dns.RS_CNAME }
func (r *RecordCNAME) GetId() string             { return r.Ref }
func (r *RecordCNAME) GetDNSName() string        { return r.Name }
func (r *RecordCNAME) GetSetIdentifier() string  { return "" }
func (r *RecordCNAME) GetValue() string          { return r.Canonical }
func (r *RecordCNAME) GetTTL() int64             { return int64(r.Ttl) }
func (r *RecordCNAME) SetTTL(ttl int64)          { r.Ttl = utils.TTLToUint32(ttl); r.UseTtl = ttl != 0 }
func (r *RecordCNAME) Copy() raw.Record          { n := *r; return &n }
func (r *RecordCNAME) PrepareUpdate() raw.Record { n := *r; n.Zone = ""; n.View = ""; return &n }

func (r *RecordCNAME) SetRoutingPolicy(setIdentifier string, policy *dns.RoutingPolicy) {}

type RecordTXT ibclient.RecordTXT

func (r *RecordTXT) GetType() string           { return dns.RS_TXT }
func (r *RecordTXT) GetId() string             { return r.Ref }
func (r *RecordTXT) GetDNSName() string        { return r.Name }
func (r *RecordTXT) GetSetIdentifier() string  { return "" }
func (r *RecordTXT) GetValue() string          { return raw.EnsureQuotedText(r.Text) }
func (r *RecordTXT) GetTTL() int64             { return int64(r.Ttl) }
func (r *RecordTXT) SetTTL(ttl int64)          { r.Ttl = utils.TTLToUint32(ttl); r.UseTtl = ttl != 0 }
func (r *RecordTXT) Copy() raw.Record          { n := *r; return &n }
func (r *RecordTXT) PrepareUpdate() raw.Record { n := *r; n.Zone = ""; n.View = ""; return &n }

func (r *RecordTXT) SetRoutingPolicy(setIdentifier string, policy *dns.RoutingPolicy) {}

var (
	_ raw.Record = (*RecordA)(nil)
	_ raw.Record = (*RecordAAAA)(nil)
	_ raw.Record = (*RecordCNAME)(nil)
	_ raw.Record = (*RecordTXT)(nil)
)

type RecordNS ibclient.RecordNS
