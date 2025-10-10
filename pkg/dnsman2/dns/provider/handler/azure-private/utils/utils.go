// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// ToAzureRecordType converts a dns.RecordType to an armprivatedns.RecordType.
func ToAzureRecordType(recordType dns.RecordType) (*armprivatedns.RecordType, error) {
	switch recordType {
	case dns.TypeA:
		return ptr.To(armprivatedns.RecordTypeA), nil
	case dns.TypeAAAA:
		return ptr.To(armprivatedns.RecordTypeAAAA), nil
	case dns.TypeCNAME:
		return ptr.To(armprivatedns.RecordTypeCNAME), nil
	case dns.TypeTXT:
		return ptr.To(armprivatedns.RecordTypeTXT), nil
	default:
		return nil, fmt.Errorf("unsupported record type %q", recordType)
	}
}

// FromAzureRecordSet converts an armprivatedns.RecordSet to a dns.RecordSet.
func FromAzureRecordSet(azureRecordSet armprivatedns.RecordSet, recordType dns.RecordType) (*dns.RecordSet, error) {
	props := azureRecordSet.Properties
	recordSet := dns.NewRecordSet(recordType, *props.TTL, nil)
	switch recordType {
	case dns.TypeA:
		if props.ARecords != nil {
			for _, record := range props.ARecords {
				recordSet.Add(&dns.Record{Value: *record.IPv4Address})
			}
		}
	case dns.TypeAAAA:
		if props.AaaaRecords != nil {
			for _, record := range props.AaaaRecords {
				recordSet.Add(&dns.Record{Value: *record.IPv6Address})
			}
		}
	case dns.TypeCNAME:
		if props.CnameRecord != nil {
			recordSet.Add(&dns.Record{Value: *props.CnameRecord.Cname})
		}
	case dns.TypeTXT:
		if props.TxtRecords != nil {
			for _, record := range props.TxtRecords {
				values := make([]string, len(record.Value))
				for i, value := range record.Value {
					values[i] = *value
				}
				value := strings.Join(values, "\n")
				if !isQuoted(value) {
					value = strconv.Quote(value)
				}
				recordSet.Add(&dns.Record{Value: value})
			}
		}
	default:
		return nil, fmt.Errorf("record type %s not supported by Azure Private DNS provider", recordType)
	}
	return recordSet, nil
}

func isQuoted(s string) bool {
	return len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"'
}
