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

package provider

import (
	"fmt"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
)

type DedicatedDNSAccess interface {
	GetRecordSet(zone DNSHostedZone, rsName dns.RecordSetName, recordType string) (DedicatedRecordSet, error)
	CreateOrUpdateRecordSet(logger logger.LogContext, zone DNSHostedZone, old, new DedicatedRecordSet) error
	DeleteRecordSet(logger logger.LogContext, zone DNSHostedZone, rs DedicatedRecordSet) error
}

type DedicatedRecord interface {
	GetType() string
	GetValue() string
	GetDNSName() string
	GetSetIdentifier() string
	GetTTL() int
}

type DedicatedRecordSet []DedicatedRecord

type dedicatedRecord struct {
	dns.RecordSetName
	Type  string
	TTL   int
	Value string
}

func (r *dedicatedRecord) GetType() string { return r.Type }

func (r *dedicatedRecord) GetValue() string { return r.Value }

func (r *dedicatedRecord) GetSetIdentifier() string { return r.SetIdentifier }

func (r *dedicatedRecord) GetDNSName() string { return r.DNSName }

func (r *dedicatedRecord) GetTTL() int { return r.TTL }

func FromDedicatedRecordSet(setName dns.RecordSetName, rs *dns.RecordSet) DedicatedRecordSet {
	recordset := DedicatedRecordSet{}
	for _, r := range rs.Records {
		recordset = append(recordset, &dedicatedRecord{
			RecordSetName: setName,
			Type:          rs.Type,
			TTL:           int(rs.TTL),
			Value:         r.Value,
		})
	}
	return recordset
}

func ToDedicatedRecordset(rawrs DedicatedRecordSet) (dns.RecordSetName, *dns.RecordSet) {
	if len(rawrs) == 0 {
		return dns.RecordSetName{}, nil
	}
	dnsName := rawrs[0].GetDNSName()
	setIdentifier := rawrs[0].GetSetIdentifier()
	rtype := rawrs[0].GetType()
	ttl := int64(rawrs[0].GetTTL())
	records := []*dns.Record{}
	for _, r := range rawrs {
		records = append(records, &dns.Record{Value: r.GetValue()})
	}
	return dns.RecordSetName{DNSName: dnsName, SetIdentifier: setIdentifier}, dns.NewRecordSet(rtype, ttl, records)
}

func (rs DedicatedRecordSet) GetAttr(name string) string {
	prefix := newAttrKeyPrefix(name)
	for _, r := range rs {
		if value := r.GetValue(); strings.HasPrefix(value, prefix) {
			return value[len(prefix) : len(value)-1]
		}
	}
	return ""
}

func newAttrKeyPrefix(name string) string {
	return fmt.Sprintf("\"%s=", name)
}
