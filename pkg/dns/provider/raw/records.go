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
	GetTTL() int
	SetTTL(int)
	Copy() Record
}

type RecordSet []Record

func (this RecordSet) Clone() RecordSet {
	clone := make(RecordSet, len(this), len(this))
	for i, r := range this {
		clone[i] = r.Copy()
	}
	return clone
}

type DNSSet map[string]RecordSet

func (this DNSSet) Clone() DNSSet {
	clone := DNSSet{}
	for rk, rv := range this {
		clone[rk] = rv.Clone()
	}
	return clone
}

type ZoneState struct {
	dnssets dns.DNSSets
	records map[dns.DNSSetName]DNSSet
}

var _ provider.DNSZoneState = &ZoneState{}

func NewState() *ZoneState {
	return &ZoneState{records: map[dns.DNSSetName]DNSSet{}}
}

func (this *ZoneState) GetDNSSets() dns.DNSSets {
	return this.dnssets
}

func (this *ZoneState) Clone() provider.DNSZoneState {
	clone := NewState()
	clone.dnssets = this.dnssets.Clone()
	clone.records = map[dns.DNSSetName]DNSSet{}
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
			e = DNSSet{}
			this.records[name] = e
		}
		e[t] = append(e[t], r)
	}
}

func (this *ZoneState) GetRecord(dnsname dns.DNSSetName, rtype, value string) Record {
	e := this.records[dnsname]
	if e != nil {
		for _, r := range e[rtype] {
			if r.GetValue() == value {
				return r
			}
		}
	}
	return nil
}

func (this *ZoneState) CalculateDNSSets() {
	this.dnssets = dns.DNSSets{}
	for dnsname, dset := range this.records {
		for rtype, rset := range dset {
			rs := dns.NewRecordSet(rtype, 0, nil)
			for _, r := range rset {
				rs.TTL = int64(r.GetTTL())
				rs.Add(&dns.Record{Value: r.GetValue()})
			}
			this.dnssets.AddRecordSetFromProviderEx(dnsname, nil, rs)
		}
	}
}

func EnsureQuotedText(v string) string {
	if _, err := strconv.Unquote(v); err != nil {
		v = strconv.Quote(v)
	}
	return v
}
