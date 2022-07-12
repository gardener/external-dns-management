/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package infoblox

import (
	ibclient "github.com/infobloxopen/infoblox-go-client/v2"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
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
func (r *RecordA) GetTTL() int              { return int(r.Ttl) }
func (r *RecordA) SetTTL(ttl int)           { r.Ttl = uint32(ttl); r.UseTtl = ttl != 0 }
func (r *RecordA) Copy() raw.Record         { n := *r; return &n }
func (r *RecordA) PrepareUpdate() raw.Record {
	n := *r
	n.Zone = ""
	n.Name = ""
	n.View = ""
	return &n
}

type RecordAAAA ibclient.RecordAAAA

func (r *RecordAAAA) GetType() string          { return dns.RS_A }
func (r *RecordAAAA) GetId() string            { return r.Ref }
func (r *RecordAAAA) GetDNSName() string       { return r.Name }
func (r *RecordAAAA) GetSetIdentifier() string { return "" }
func (r *RecordAAAA) GetValue() string         { return r.Ipv6Addr }
func (r *RecordAAAA) GetTTL() int              { return int(r.Ttl) }
func (r *RecordAAAA) SetTTL(ttl int)           { r.Ttl = uint32(ttl); r.UseTtl = ttl != 0 }
func (r *RecordAAAA) Copy() raw.Record         { n := *r; return &n }
func (r *RecordAAAA) PrepareUpdate() raw.Record {
	n := *r
	n.Zone = ""
	n.Name = ""
	n.View = ""
	return &n
}

type RecordCNAME ibclient.RecordCNAME

func (r *RecordCNAME) GetType() string           { return dns.RS_CNAME }
func (r *RecordCNAME) GetId() string             { return r.Ref }
func (r *RecordCNAME) GetDNSName() string        { return r.Name }
func (r *RecordCNAME) GetSetIdentifier() string  { return "" }
func (r *RecordCNAME) GetValue() string          { return r.Canonical }
func (r *RecordCNAME) GetTTL() int               { return int(r.Ttl) }
func (r *RecordCNAME) SetTTL(ttl int)            { r.Ttl = uint32(ttl); r.UseTtl = ttl != 0 }
func (r *RecordCNAME) Copy() raw.Record          { n := *r; return &n }
func (r *RecordCNAME) PrepareUpdate() raw.Record { n := *r; n.Zone = ""; n.View = ""; return &n }

type RecordTXT ibclient.RecordTXT

func (r *RecordTXT) GetType() string           { return dns.RS_TXT }
func (r *RecordTXT) GetId() string             { return r.Ref }
func (r *RecordTXT) GetDNSName() string        { return r.Name }
func (r *RecordTXT) GetSetIdentifier() string  { return "" }
func (r *RecordTXT) GetValue() string          { return raw.EnsureQuotedText(r.Text) }
func (r *RecordTXT) GetTTL() int               { return int(r.Ttl) }
func (r *RecordTXT) SetTTL(ttl int)            { r.Ttl = uint(ttl); r.UseTtl = ttl != 0 }
func (r *RecordTXT) Copy() raw.Record          { n := *r; return &n }
func (r *RecordTXT) PrepareUpdate() raw.Record { n := *r; n.Zone = ""; n.View = ""; return &n }

var _ raw.Record = (*RecordA)(nil)
var _ raw.Record = (*RecordAAAA)(nil)
var _ raw.Record = (*RecordCNAME)(nil)
var _ raw.Record = (*RecordTXT)(nil)

type RecordNS ibclient.RecordNS
