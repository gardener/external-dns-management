// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package records

import (
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// FullDNSSetName represents a DNS set name with its associated zone.
type FullDNSSetName struct {
	ZoneID dns.ZoneID
	Name   dns.DNSSetName
}

// FullRecordSetKey represents a unique key for a DNS record set, including zone, name, and record type.
type FullRecordSetKey struct {
	FullDNSSetName
	RecordType dns.RecordType
}

// FullRecordKeySet is a set of FullRecordSetKey.
type FullRecordKeySet = sets.Set[FullRecordSetKey]

// InsertRecordKeys inserts record keys for the given targets into the provided key set.
func InsertRecordKeys(keys FullRecordKeySet, fullDNSSetName FullDNSSetName, targets dns.Targets) {
	for _, target := range targets {
		keys.Insert(FullRecordSetKey{
			FullDNSSetName: fullDNSSetName,
			RecordType:     target.GetRecordType(),
		})
	}
}

// InsertRecordSets inserts record sets for the given targets and routing policy into the provided DNSSet.
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
