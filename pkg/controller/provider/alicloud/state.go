/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package alicloud

import (
	"fmt"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"strings"
)

type RecordSet []alidns.Record
type DNSSet map[string]RecordSet

type zonestate struct {
	dnssets dns.DNSSets
	records map[string]DNSSet
}

var _ provider.DNSZoneState = &zonestate{}

func newState() *zonestate {
	return &zonestate{records: map[string]DNSSet{}}
}

func (this *zonestate) GetDNSSets() dns.DNSSets {
	return this.dnssets
}

func (this *zonestate) addRecord(r alidns.Record) {
	if dns.SupportedRecordType(r.Type) {
		name := GetDNSName(r)
		e := this.records[name]
		if e == nil {
			e = DNSSet{}
			this.records[name] = e
		}
		r = unescape(r)
		e[r.Type] = append(e[r.Type], r)
	}
}

func (this *zonestate) getRecord(dnsname, rtype, value string) *alidns.Record {
	e := this.records[dnsname]
	if e != nil {
		for _, r := range e[rtype] {
			if r.Value == value {
				return &r
			}
		}
	}
	return nil
}

func (this *zonestate) calculateDNSSets() {
	this.dnssets = dns.DNSSets{}
	for dnsname, dset := range this.records {
		for rtype, rset := range dset {
			rs := dns.NewRecordSet(rtype, int64(rset[0].TTL), nil)
			for _, r := range rset {
				rs.Add(&dns.Record{Value: r.Value})
			}
			this.dnssets.AddRecordSetFromProvider(dnsname, rs)
		}
	}
}

func unescape(r alidns.Record) alidns.Record {
	if r.Type == dns.RS_TXT && !strings.HasPrefix(r.Value, "\"") {
		r.Value = fmt.Sprintf("\"%s\"", r.Value)
	}
	return r
}
