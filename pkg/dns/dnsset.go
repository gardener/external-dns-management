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

package dns

import (
	"github.com/gardener/controller-manager-library/pkg/utils"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

////////////////////////////////////////////////////////////////////////////////
// A DNSSet contains Record sets for an DNS name. The name is given without
// trailing dot. If the provider required this dot, it must be removed or addeed
// whe reading or writing recordsets, respectively.
// Supported record set types are:
// - TXT
// - CNAME
// - A
// - META   virtual type used by this API (see below) to store meta data
//
// If multiple CNAME records are given they will be mapped to A records
// by resolving the cnames. THis resolution will be updated periodically,
//
// The META records contain attribute settings of the form "<attr>=<value>".
// They are used to store the identifier of the controller and other
// meta data to identity sets maintained or owned by this controller.
// This record set must be stored and restored by the provider in some
//  applicable way.
//
// This library supports a default mechanics for ths task, that can be used by
// the provider:
// This record set always contains a prefix attribute used to map META
// records to TXT records finally stored by the provider.
// Because not all regular record types can be combined with TXT records
// the META text records are stores for a separate dns name composed of
// the prefix and the original name.
// This mapping is done by the the two functions MapFromProvider and
// MapToProvider. These methods can be called by the provider when reading
// or writing a record set, respectively. The map the given set to
// an effective set and dns name for the desired purpose.

type DNSSets map[DNSSetName]*DNSSet

type Ownership interface {
	IsResponsibleFor(id string) bool
	GetIds() utils.StringSet
}

func (dnssets DNSSets) AddRecordSetFromProvider(dnsName string, rs *RecordSet) {
	dnssets.AddRecordSetFromProviderEx(DNSSetName{DNSName: dnsName}, nil, rs)
}

func (dnssets DNSSets) AddRecordSetFromProviderEx(setName DNSSetName, policy *RoutingPolicy, rs *RecordSet) {
	name := setName.Normalize()
	name, rs = MapFromProvider(name, rs)

	dnssets.AddRecordSet(name, policy, rs)
}

func (dnssets DNSSets) AddRecordSet(name DNSSetName, policy *RoutingPolicy, rs *RecordSet) {
	dnsset := dnssets[name]
	if dnsset == nil {
		dnsset = NewDNSSet(name, policy)
		dnssets[name] = dnsset
	}
	dnsset.Sets[rs.Type] = rs
	if rs.Type == RS_CNAME {
		for i := range rs.Records {
			rs.Records[i].Value = NormalizeHostname(rs.Records[i].Value)
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

// GetOwners returns all owners for all DNSSets
func (dnssets DNSSets) GetOwners() utils.StringSet {
	owners := utils.NewStringSet()
	for _, dnsset := range dnssets {
		o := dnsset.GetMetaAttr(ATTR_OWNER)
		if o != "" {
			owners.Add(o)
		}
	}
	return owners
}

const (
	ATTR_OWNER  = "owner"
	ATTR_PREFIX = "prefix"
	ATTR_KIND   = "kind"

	ATTR_TIMESTAMP = "ts"
	ATTR_LOCKID    = "lockid"
)

type DNSSet struct {
	Name          DNSSetName
	Kind          string
	UpdateGroup   string
	Sets          RecordSets
	RoutingPolicy *RoutingPolicy
}

func (this *DNSSet) Clone() *DNSSet {
	return &DNSSet{
		Name: this.Name, Sets: this.Sets.Clone(), UpdateGroup: this.UpdateGroup, Kind: this.Kind,
		RoutingPolicy: this.RoutingPolicy.Clone(),
	}
}

func (this *DNSSet) getAttr(ty string, name string) string {
	rset := this.Sets[ty]
	if rset != nil {
		return rset.GetAttr(name)
	}
	return ""
}

func (this *DNSSet) setAttr(ty string, name string, value string) {
	rset := this.Sets[ty]
	if rset == nil {
		rset = newAttrRecordSet(ty, name, value)
		this.Sets[rset.Type] = rset
	} else {
		rset.SetAttr(name, value)
	}
}

func (this *DNSSet) deleteAttr(ty string, name string) {
	rset := this.Sets[ty]
	if rset != nil {
		rset.DeleteAttr(name)
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

func (this *DNSSet) GetMetaAttr(name string) string {
	return this.getAttr(RS_META, name)
}

func (this *DNSSet) SetMetaAttr(name string, value string) {
	this.setAttr(RS_META, name, value)
}

func (this *DNSSet) DeleteMetaAttr(name string) {
	this.deleteAttr(RS_META, name)
}

func (this *DNSSet) IsOwnedBy(ownership Ownership) bool {
	o := this.GetMetaAttr(ATTR_OWNER)
	return o != "" && ownership.IsResponsibleFor(o)
}

func (this *DNSSet) IsForeign(ownership Ownership) bool {
	o := this.GetMetaAttr(ATTR_OWNER)
	return o != "" && !ownership.IsResponsibleFor(o)
}

func (this *DNSSet) GetOwner() string {
	return this.GetMetaAttr(ATTR_OWNER)
}

func (this *DNSSet) SetOwner(ownerid string) *DNSSet {
	this.SetMetaAttr(ATTR_OWNER, ownerid)
	return this
}

func (this *DNSSet) GetKind() string {
	if this.Kind == "" {
		this.Kind = this.GetMetaAttr(ATTR_KIND)
	}
	if this.Kind == "" {
		this.Kind = api.DNSEntryKind
	}
	return this.Kind
}

func (this *DNSSet) SetKind(t string, prop ...bool) *DNSSet {
	this.Kind = t
	if t != api.DNSEntryKind {
		if len(prop) == 0 || prop[0] {
			this.SetMetaAttr(ATTR_KIND, t)
		}
	} else {
		this.DeleteMetaAttr(ATTR_KIND)
	}
	return this
}

func (this *DNSSet) SetRecordSet(rtype string, ttl int64, values ...string) {
	records := make([]*Record, len(values))
	for i, r := range values {
		records[i] = &Record{Value: r}
	}
	this.Sets[rtype] = &RecordSet{Type: rtype, TTL: ttl, IgnoreTTL: false, Records: records}
}

func NewDNSSet(name DNSSetName, routingPolicy *RoutingPolicy) *DNSSet {
	return &DNSSet{Name: name, RoutingPolicy: routingPolicy, Sets: map[string]*RecordSet{}}
}
