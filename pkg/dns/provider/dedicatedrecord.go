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
	GetRecordSet(zone DNSHostedZone, dnsName, recordType string) (DedicatedRecordSet, error)
	CreateOrUpdateRecordSet(logger logger.LogContext, zone DNSHostedZone, old, new DedicatedRecordSet) error
	DeleteRecordSet(logger logger.LogContext, zone DNSHostedZone, rs DedicatedRecordSet) error
}

type DedicatedRecord interface {
	GetType() string
	GetValue() string
	GetDNSName() string
	GetTTL() int
}

type DedicatedRecordSet []DedicatedRecord

type dedicatedRecord struct {
	DNSName string
	Type    string
	TTL     int
	Value   string
}

func (r *dedicatedRecord) GetType() string { return r.Type }

func (r *dedicatedRecord) GetValue() string { return r.Value }

func (r *dedicatedRecord) GetDNSName() string { return r.DNSName }

func (r *dedicatedRecord) GetTTL() int { return r.TTL }

func FromDedicatedRecordSet(dnsName string, rs *dns.RecordSet) DedicatedRecordSet {
	recordset := DedicatedRecordSet{}
	for _, r := range rs.Records {
		recordset = append(recordset, &dedicatedRecord{
			DNSName: dnsName,
			Type:    rs.Type,
			TTL:     int(rs.TTL),
			Value:   r.Value,
		})
	}
	return recordset
}

func ToDedicatedRecordset(rawrs DedicatedRecordSet) (string, *dns.RecordSet) {
	if len(rawrs) == 0 {
		return "", nil
	}
	dnsName := rawrs[0].GetDNSName()
	rtype := rawrs[0].GetType()
	ttl := int64(rawrs[0].GetTTL())
	records := []*dns.Record{}
	for _, r := range rawrs {
		records = append(records, &dns.Record{Value: r.GetValue()})
	}
	return dnsName, dns.NewRecordSet(rtype, ttl, records)
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
