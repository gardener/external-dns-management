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
	GetTTL() int
	SetTTL(int)
	Copy() Record
}

type RecordSet []Record
type DNSSet map[string]RecordSet

type ZoneState struct {
	dnssets dns.DNSSets
	records map[string]DNSSet
}

var _ provider.DNSZoneState = &ZoneState{}

func NewState() *ZoneState {
	return &ZoneState{records: map[string]DNSSet{}}
}

func (this *ZoneState) GetDNSSets() dns.DNSSets {
	return this.dnssets
}

func (this *ZoneState) AddRecord(r Record) {
	if dns.SupportedRecordType(r.GetType()) {
		name := r.GetDNSName()
		t := r.GetType()
		e := this.records[name]
		if e == nil {
			e = DNSSet{}
			this.records[name] = e
		}
		e[t] = append(e[t], r)
	}
}

func (this *ZoneState) GetRecord(dnsname, rtype, value string) Record {
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
				rs.Add(&dns.Record{Value: r.GetValue()})
			}
			this.dnssets.AddRecordSetFromProvider(dnsname, rs)
		}
	}
}

func EnsureQuotedText(v string) string {
	if _, err := strconv.Unquote(v); err != nil {
		v = strconv.Quote(v)
	}
	return v
}
