// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package alicloud

import (
	alidns "github.com/alibabacloud-go/alidns-20150109/v4/client"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Record alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord

var _ raw.Record = &Record{}

func (r *Record) GetType() string          { return ptr.Deref(r.Type, "") }
func (r *Record) GetId() string            { return ptr.Deref(r.RecordId, "") }
func (r *Record) GetDNSName() string {
	return GetDNSName(alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord(*r))
}
func (r *Record) GetSetIdentifier() string { return "" }
func (r *Record) GetValue() string {
	v := ptr.Deref(r.Value, "")
	if ptr.Deref(r.Type, "") == dns.RS_TXT {
		return raw.EnsureQuotedText(v)
	}
	return v
}
func (r *Record) GetTTL() int64    { return ptr.Deref(r.TTL, 0) }
func (r *Record) SetTTL(ttl int64) { r.TTL = ptr.To(ttl) }
func (r *Record) Copy() raw.Record { n := *r; return &n }
