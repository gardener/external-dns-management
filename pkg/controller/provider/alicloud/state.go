// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package alicloud

import (
	"strconv"
	"strings"

	alidns "github.com/alibabacloud-go/alidns-20150109/v5/client"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Record alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord

var _ raw.Record = &Record{}

const (
	routingPolicySetRemarkPrefix = "routing-policy-set-"
	deleteRemark                 = "<delete-remark>"
)

func (r *Record) GetType() string { return ptr.Deref(r.Type, "") }
func (r *Record) GetId() string   { return ptr.Deref(r.RecordId, "") }
func (r *Record) GetDNSName() string {
	return GetDNSName(alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord(*r))
}
func (r *Record) GetSetIdentifier() string {
	remark := ptr.Deref(r.Remark, "")
	if remark == "" {
		return ""
	}
	if !strings.HasPrefix(remark, routingPolicySetRemarkPrefix) {
		return ""
	}
	return strings.TrimPrefix(remark, routingPolicySetRemarkPrefix)
}

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

func (r *Record) SetRoutingPolicy(setIdentifier string, policy *dns.RoutingPolicy) {
	if setIdentifier == "" || policy == nil || policy.Type != dns.RoutingPolicyWeighted {
		if r.Remark != nil {
			s := deleteRemark
			r.Remark = &s
		} else {
			r.Remark = nil
		}
		r.Weight = nil
		return
	}

	remark := routingPolicySetRemarkPrefix + setIdentifier
	r.Remark = ptr.To(remark)
	var weight int32 = 1
	if w := policy.Parameters["weight"]; w != "" {
		if v, err := strconv.Atoi(w); err == nil && v >= 1 && v <= 100 {
			weight = int32(v) // #nosec G402 G115 G109 -- only values between 1 and 100
		}
	}
	r.Weight = ptr.To(weight)
}
