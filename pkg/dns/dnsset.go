// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

////////////////////////////////////////////////////////////////////////////////
// A DNSSet contains record sets for a DNS name. The name is given without
// trailing dot. If the provider requires this dot, it must be removed or added
// whe reading or writing record sets, respectively.
// Supported record set types are:
// - TXT
// - CNAME
// - A
// - AAAA
//
// If multiple CNAME records are given they will be mapped to A records
// by resolving the cnames. This resolution will be updated periodically.

type DNSSets map[DNSSetName]*DNSSet

type Ownership interface {
	IsResponsibleFor(id string) bool
	GetIds() utils.StringSet
}

func (dnssets DNSSets) AddRecordSetFromProvider(dnsName string, recordSet *RecordSet) {
	dnssets.AddRecordSetFromProviderEx(DNSSetName{DNSName: dnsName}, nil, recordSet)
}

func (dnssets DNSSets) AddRecordSetFromProviderEx(setName DNSSetName, policy *RoutingPolicy, recordSet *RecordSet) {
	dnssets.AddRecordSet(setName.Normalize(), policy, recordSet)
}

func (dnssets DNSSets) AddRecordSet(name DNSSetName, policy *RoutingPolicy, recordSet *RecordSet) {
	dnsset := dnssets[name]
	if dnsset == nil {
		dnsset = NewDNSSet(name, policy)
		dnssets[name] = dnsset
	}
	dnsset.Sets[recordSet.Type] = recordSet
	if recordSet.Type == RS_CNAME {
		for i := range recordSet.Records {
			recordSet.Records[i].Value = NormalizeHostname(recordSet.Records[i].Value)
		}
	}
	dnsset.RoutingPolicy = policy
}

func (dnssets DNSSets) RemoveRecordSet(name DNSSetName, recordSetType string) {
	dnsset := dnssets[name]
	if dnsset != nil {
		delete(dnsset.Sets, recordSetType)
		if len(dnsset.Sets) == 0 {
			delete(dnssets, name)
		}
	}
}

func (dnssets DNSSets) Clone() DNSSets {
	clone := DNSSets{}
	for dk, dv := range dnssets {
		clone[dk] = dv.Clone()
	}
	return clone
}

type DNSSet struct {
	Name          DNSSetName
	UpdateGroup   string
	Sets          RecordSets
	RoutingPolicy *RoutingPolicy
}

func (this *DNSSet) Clone() *DNSSet {
	return &DNSSet{
		Name: this.Name, Sets: this.Sets.Clone(), UpdateGroup: this.UpdateGroup,
		RoutingPolicy: this.RoutingPolicy.Clone(),
	}
}

func (this *DNSSet) getAttr(recordType string, name string) string {
	recordSet := this.Sets[recordType]
	if recordSet != nil {
		return recordSet.GetAttr(name)
	}
	return ""
}

func (this *DNSSet) setAttr(recordType string, name string, value string) {
	recordSet := this.Sets[recordType]
	if recordSet == nil {
		recordSet = newAttrRecordSet(recordType, name, value)
		this.Sets[recordSet.Type] = recordSet
	} else {
		recordSet.SetAttr(name, value)
	}
}

func (this *DNSSet) deleteAttr(recordType string, name string) {
	recordSet := this.Sets[recordType]
	if recordSet != nil {
		recordSet.DeleteAttr(name)
	}
}

func (this *DNSSet) GetTxtAttr(name string) string {
	return this.getAttr(RS_TXT, name)
}

func (this *DNSSet) SetTxtAttr(name string, value string) {
	this.setAttr(RS_TXT, name, value)
}

func (this *DNSSet) DeleteTxtAttr(name string) {
	this.deleteAttr(RS_TXT, name)
}

func (this *DNSSet) SetRecordSet(recordType string, ttl int64, values ...string) {
	records := make([]*Record, len(values))
	for i, r := range values {
		records[i] = &Record{Value: r}
	}
	this.Sets[recordType] = &RecordSet{Type: recordType, TTL: ttl, IgnoreTTL: false, Records: records}
}

func NewDNSSet(name DNSSetName, routingPolicy *RoutingPolicy) *DNSSet {
	return &DNSSet{Name: name, RoutingPolicy: routingPolicy, Sets: map[string]*RecordSet{}}
}

// Match matches DNSSet equality
func (this *DNSSet) Match(that *DNSSet) bool {
	return this.match(that, nil)
}

// MatchRecordTypeSubset matches DNSSet equality for given record type subset.
func (this *DNSSet) MatchRecordTypeSubset(that *DNSSet, recordType string) bool {
	return this.match(that, &recordType)
}

func (this *DNSSet) match(that *DNSSet, restrictToRecordType *string) bool {
	if this == that {
		return true
	}
	if this == nil || that == nil {
		return false
	}
	if this.Name != that.Name {
		return false
	}
	if this.UpdateGroup != that.UpdateGroup {
		return false
	}
	if this.RoutingPolicy != that.RoutingPolicy && !reflect.DeepEqual(this.RoutingPolicy, that.RoutingPolicy) {
		return false
	}
	if restrictToRecordType != nil {
		rs1, rs2 := this.Sets[*restrictToRecordType], that.Sets[*restrictToRecordType]
		if rs1 != nil && rs2 != nil {
			return rs1.Match(rs2)
		}
		return rs1 == nil && rs2 == nil
	}

	if len(this.Sets) != len(that.Sets) {
		return false
	}
	for k, v := range this.Sets {
		w := that.Sets[k]
		if w == nil {
			return false
		}
		if !v.Match(w) {
			return false
		}
	}
	return true
}
