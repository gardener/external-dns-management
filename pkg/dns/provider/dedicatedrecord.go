// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"

	"github.com/gardener/external-dns-management/pkg/dns"
)

type DedicatedDNSAccess interface {
	GetRecordSet(zone DNSHostedZone, name dns.DNSSetName, recordType string) (DedicatedRecordSet, error)
	CreateOrUpdateRecordSet(logger logger.LogContext, zone DNSHostedZone, old, new DedicatedRecordSet) error
	DeleteRecordSet(logger logger.LogContext, zone DNSHostedZone, rs DedicatedRecordSet) error
}

type DedicatedRecord interface {
	GetType() string
	GetValue() string
	GetDNSName() string
	GetSetIdentifier() string
	GetTTL() int64
}

type DedicatedRecordSet []DedicatedRecord

type dedicatedRecord struct {
	dns.DNSSetName
	Type  string
	TTL   int64
	Value string
}

func (r *dedicatedRecord) GetType() string { return r.Type }

func (r *dedicatedRecord) GetValue() string { return r.Value }

func (r *dedicatedRecord) GetSetIdentifier() string { return r.SetIdentifier }

func (r *dedicatedRecord) GetDNSName() string { return r.DNSName }

func (r *dedicatedRecord) GetTTL() int64 { return r.TTL }

func FromDedicatedRecordSet(setName dns.DNSSetName, rs *dns.RecordSet) DedicatedRecordSet {
	recordset := DedicatedRecordSet{}
	for _, r := range rs.Records {
		recordset = append(recordset, &dedicatedRecord{
			DNSSetName: setName,
			Type:       rs.Type,
			TTL:        rs.TTL,
			Value:      r.Value,
		})
	}
	return recordset
}

func ToDedicatedRecordset(rawrs DedicatedRecordSet) (dns.DNSSetName, *dns.RecordSet) {
	if len(rawrs) == 0 {
		return dns.DNSSetName{}, nil
	}
	dnsName := rawrs[0].GetDNSName()
	setIdentifier := rawrs[0].GetSetIdentifier()
	rtype := rawrs[0].GetType()
	ttl := rawrs[0].GetTTL()
	records := []*dns.Record{}
	for _, r := range rawrs {
		records = append(records, &dns.Record{Value: r.GetValue()})
	}
	return dns.DNSSetName{DNSName: dnsName, SetIdentifier: setIdentifier}, dns.NewRecordSet(rtype, ttl, records)
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
