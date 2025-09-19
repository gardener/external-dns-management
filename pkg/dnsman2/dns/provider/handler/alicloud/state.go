// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package alicloud

import (
	"strconv"
	"strings"

	alidns "github.com/alibabacloud-go/alidns-20150109/v4/client"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/raw"
)

// Record is a wrapper around alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord
type Record alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord

var _ raw.Record = &Record{}

const (
	routingPolicySetRemarkPrefix = "routing-policy-set-"
	deleteRemark                 = "<delete-remark>"
)

// GetType returns the record type.
func (r *Record) GetType() string { return ptr.Deref(r.Type, "") }

// GetId returns the record ID.
func (r *Record) GetId() string { return ptr.Deref(r.RecordId, "") }

// GetDNSName returns the fully qualified domain name of the record.
func (r *Record) GetDNSName() string {
	return GetDNSName(alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord(*r))
}

// GetSetIdentifier returns the set identifier of the record (if any).
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

// GetValue returns the value of the record.
func (r *Record) GetValue() string {
	v := ptr.Deref(r.Value, "")
	if ptr.Deref(r.Type, "") == string(dns.TypeTXT) {
		return raw.EnsureQuotedText(v)
	}
	return v
}

// GetTTL returns the TTL of the record.
func (r *Record) GetTTL() int64 { return ptr.Deref(r.TTL, 0) }

// SetTTL sets the TTL of the record.
func (r *Record) SetTTL(ttl int64) { r.TTL = ptr.To(ttl) }

// Clone returns a deep copy of the record.
func (r *Record) Clone() raw.Record { n := *r; return &n }

// SetRoutingPolicy sets the routing policy of the record.
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
