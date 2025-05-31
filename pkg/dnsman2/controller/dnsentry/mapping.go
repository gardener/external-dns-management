// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"sort"
	"strings"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// EntryRecordSet is a struct that combines a DNS RecordSet with its type and name.
type EntryRecordSet struct {
	dns.RecordSet
	Type dns.RecordType
	Name dns.DNSSetName
}

func MapSpecToRecordSets(entry *v1alpha1.DNSEntry) []EntryRecordSet {
	name := dns.DNSSetName{
		DNSName:       entry.Spec.DNSName,
		SetIdentifier: "", // TODO(MartinWeindel): handle set identifiers for routing policies
	}.Normalize()
	recordSets := map[dns.RecordType]dns.RecordSet{}

	result := make([]EntryRecordSet, 0, len(recordSets))
	for t, rs := range recordSets {
		result = append(result, EntryRecordSet{
			RecordSet: rs,
			Type:      t,
			Name:      name,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.Compare(string(result[i].Type), string(result[j].Type)) < 0
	})
	return result
}
