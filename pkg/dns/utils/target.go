// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"

	"github.com/gardener/external-dns-management/pkg/dns"
)

////////////////////////////////////////////////////////////////////////////////
// DNS Target
////////////////////////////////////////////////////////////////////////////////

type Targets []Target

func (this Targets) Has(target Target) bool {
	for _, t := range this {
		if t.GetRecordType() == target.GetRecordType() &&
			t.GetHostName() == target.GetHostName() &&
			t.GetIPStack() == target.GetIPStack() {
			return true
		}
	}
	return false
}

func (this Targets) DifferFrom(targets Targets) bool {
	if len(this) != len(targets) {
		return true
	}
	for _, t := range this {
		if !targets.Has(t) {
			return true
		}
	}
	return false
}

type Target interface {
	GetHostName() string
	GetRecordType() string
	GetTTL() int64
	AsRecord() *dns.Record
	GetIPStack() string
}

type target struct {
	rtype   string
	host    string
	ttl     int64
	ipstack string
}

func NewText(t string, ttl int64) Target {
	return NewTarget(dns.RS_TXT, fmt.Sprintf("%q", t), ttl)
}

func NewTarget(ty string, ta string, ttl int64) Target {
	return &target{rtype: ty, host: ta, ttl: ttl}
}

func NewTargetWithIPStack(ty string, ta string, ttl int64, ipstack string) Target {
	return &target{rtype: ty, host: ta, ttl: ttl, ipstack: ipstack}
}

func (t *target) GetTTL() int64         { return t.ttl }
func (t *target) GetHostName() string   { return t.host }
func (t *target) GetRecordType() string { return t.rtype }
func (t *target) GetIPStack() string    { return t.ipstack }

func (t *target) AsRecord() *dns.Record {
	return &dns.Record{Value: t.host}
}

func (t *target) String() string {
	return fmt.Sprintf("%s(%s)", t.GetRecordType(), t.GetHostName())
}

////////////////////////////////////////////////////////////////////////////////
// DNS Target Spec
////////////////////////////////////////////////////////////////////////////////

type TargetSpec interface {
	Kind() string
	OwnerId() string
	Targets() []Target
	RoutingPolicy() *dns.RoutingPolicy
	Responsible(set *dns.DNSSet, ownership dns.Ownership) bool
}

type targetSpec struct {
	kind          string
	ownerId       string
	targets       []Target
	routingPolicy *dns.RoutingPolicy
}

func BaseTargetSpec(entry *DNSEntryObject, p TargetProvider) TargetSpec {
	spec := &targetSpec{
		kind:          entry.GroupKind().Kind,
		ownerId:       p.OwnerId(),
		targets:       p.Targets(),
		routingPolicy: p.RoutingPolicy(),
	}
	return spec
}

func (this *targetSpec) Kind() string {
	return this.kind
}

func (this *targetSpec) OwnerId() string {
	return this.ownerId
}

func (this *targetSpec) Targets() []Target {
	return this.targets
}

func (this *targetSpec) Responsible(set *dns.DNSSet, ownership dns.Ownership) bool {
	return !set.IsForeign(ownership)
}

func (this *targetSpec) RoutingPolicy() *dns.RoutingPolicy {
	return this.routingPolicy
}
