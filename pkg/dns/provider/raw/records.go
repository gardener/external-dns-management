// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package raw

import (
	"strconv"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Record interface {
	GetId() string
	GetType() string
	GetValue() string
	GetDNSName() string
	GetSetIdentifier() string
	GetTTL() int64
	SetTTL(int64)
	SetRoutingPolicy(setIdentifier string, policy *dns.RoutingPolicy)
	Copy() Record
}

type RecordSet []Record

func (this RecordSet) Clone() RecordSet {
	clone := make(RecordSet, len(this))
	for i, r := range this {
		clone[i] = r.Copy()
	}
	return clone
}

type dnsSet struct {
	records       map[string]RecordSet
	routingPolicy *dns.RoutingPolicy
}

func newDNSSet() *dnsSet {
	return &dnsSet{
		records:       map[string]RecordSet{},
		routingPolicy: nil,
	}
}

func (s *dnsSet) Clone() *dnsSet {
	clone := newDNSSet()
	for rk, rv := range s.records {
		clone.records[rk] = rv.Clone()
	}
	if s.routingPolicy != nil {
		clone.routingPolicy = s.routingPolicy.Clone()
	}
	return clone
}

type ZoneState struct {
	dnssets dns.DNSSets
	records map[dns.DNSSetName]*dnsSet
}

var _ provider.DNSZoneState = &ZoneState{}

func NewState() *ZoneState {
	return &ZoneState{records: map[dns.DNSSetName]*dnsSet{}}
}

func (this *ZoneState) GetDNSSets() dns.DNSSets {
	return this.dnssets
}

func (this *ZoneState) Clone() provider.DNSZoneState {
	clone := NewState()
	clone.dnssets = this.dnssets.Clone()
	clone.records = map[dns.DNSSetName]*dnsSet{}
	for k, v := range this.records {
		clone.records[k] = v.Clone()
	}
	return clone
}

func (this *ZoneState) AddRecord(r Record) {
	if dns.SupportedRecordType(r.GetType()) {
		name := dns.DNSSetName{DNSName: r.GetDNSName(), SetIdentifier: r.GetSetIdentifier()}
		t := r.GetType()
		e := this.records[name]
		if e == nil {
			e = newDNSSet()
			this.records[name] = e
		}
		e.records[t] = append(e.records[t], r)
	}
}

func (this *ZoneState) AddRecordWithRoutingPolicy(r Record, policy *dns.RoutingPolicy) {
	if dns.SupportedRecordType(r.GetType()) {
		name := dns.DNSSetName{DNSName: r.GetDNSName(), SetIdentifier: r.GetSetIdentifier()}
		t := r.GetType()
		e := this.records[name]
		if e == nil {
			e = newDNSSet()
			this.records[name] = e
		}
		e.records[t] = append(e.records[t], r)
		e.routingPolicy = policy
	}
}

func (this *ZoneState) GetRecord(dnsname dns.DNSSetName, rtype, value string) Record {
	e := this.records[dnsname]
	if e != nil {
		for _, r := range e.records[rtype] {
			if r.GetValue() == value {
				return r
			}
		}
	}
	return nil
}

func (this *ZoneState) GetRoutingPolicy(dnsname dns.DNSSetName) *dns.RoutingPolicy {
	e := this.records[dnsname]
	if e != nil {
		return e.routingPolicy
	}
	return nil
}

func (this *ZoneState) CalculateDNSSets() {
	this.dnssets = dns.DNSSets{}
	for dnsname, dset := range this.records {
		for rtype, rset := range dset.records {
			rs := dns.NewRecordSet(rtype, 0, nil)
			for _, r := range rset {
				rs.TTL = r.GetTTL()
				rs.Add(&dns.Record{Value: r.GetValue()})
			}
			this.dnssets.AddRecordSetFromProviderEx(dnsname, dset.routingPolicy, rs)
		}
	}
}

func EnsureQuotedText(v string) string {
	if _, err := strconv.Unquote(v); err != nil {
		v = strconv.Quote(v)
	}
	return v
}
