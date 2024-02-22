// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package alicloud

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Record alidns.Record

var _ raw.Record = &Record{}

func (r *Record) GetType() string          { return r.Type }
func (r *Record) GetId() string            { return r.RecordId }
func (r *Record) GetDNSName() string       { return GetDNSName(alidns.Record(*r)) }
func (r *Record) GetSetIdentifier() string { return "" }
func (r *Record) GetValue() string {
	if r.Type == dns.RS_TXT {
		return raw.EnsureQuotedText(r.Value)
	}
	return r.Value
}
func (r *Record) GetTTL() int      { return r.TTL }
func (r *Record) SetTTL(ttl int)   { r.TTL = ttl }
func (r *Record) Copy() raw.Record { n := *r; return &n }
