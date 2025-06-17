// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package records

import (
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

type FullDNSSetName struct {
	ZoneID dns.ZoneID
	Name   dns.DNSSetName
}

type FullRecordSetKey struct {
	FullDNSSetName
	RecordType dns.RecordType
}

type FullRecordKeySet = sets.Set[FullRecordSetKey]

func InsertRecordKeys(keys FullRecordKeySet, fullDNSSetName FullDNSSetName, targets dns.Targets) {
	for _, target := range targets {
		keys.Insert(FullRecordSetKey{
			FullDNSSetName: fullDNSSetName,
			RecordType:     target.GetRecordType(),
		})
	}
}

func InsertRecordSets(dnsSet dns.DNSSet, policy *dns.RoutingPolicy, targets dns.Targets) {
	recordSets := map[dns.RecordType]*dns.RecordSet{}
	for _, target := range targets {
		if _, exists := recordSets[target.GetRecordType()]; !exists {
			recordSets[target.GetRecordType()] = &dns.RecordSet{
				Type:          target.GetRecordType(),
				TTL:           target.GetTTL(),
				RoutingPolicy: policy,
			}
		}
		recordSets[target.GetRecordType()].Records = append(recordSets[target.GetRecordType()].Records, target.AsRecord())
	}
	for rtype, recordSet := range recordSets {
		dnsSet.Sets[rtype] = recordSet
	}
}
