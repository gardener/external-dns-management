// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"fmt"
)

type TargetProvider interface {
	Targets() Targets
	TTL() int64
	RoutingPolicy() *RoutingPolicy
}

////////////////////////////////////////////////////////////////////////////////
// DNS Target
////////////////////////////////////////////////////////////////////////////////

type Targets []Target

func (t Targets) Has(target Target) bool {
	for _, t := range t {
		if t.GetRecordType() == target.GetRecordType() &&
			t.GetRecordValue() == target.GetRecordValue() &&
			t.GetIPStack() == target.GetIPStack() &&
			t.GetTTL() == target.GetTTL() {
			return true
		}
	}
	return false
}

func (t Targets) DifferFrom(targets Targets) bool {
	if len(t) != len(targets) {
		return true
	}
	for _, t := range t {
		if !targets.Has(t) {
			return true
		}
	}
	return false
}

type Target interface {
	GetRecordValue() string
	GetRecordType() RecordType
	GetTTL() int64
	AsRecord() *Record
	GetIPStack() string
}

type target struct {
	rtype   RecordType
	value   string
	ttl     int64
	ipstack string
}

func NewText(t string, ttl int64) Target {
	return NewTarget(TypeTXT, t, ttl)
}

func NewTarget(rtype RecordType, ta string, ttl int64) Target {
	return &target{rtype: rtype, value: ta, ttl: ttl}
}

func NewTargetWithIPStack(rtype RecordType, ta string, ttl int64, ipstack string) Target {
	return &target{rtype: rtype, value: ta, ttl: ttl, ipstack: ipstack}
}

func (t *target) GetTTL() int64             { return t.ttl }
func (t *target) GetRecordValue() string    { return t.value }
func (t *target) GetRecordType() RecordType { return t.rtype }
func (t *target) GetIPStack() string        { return t.ipstack }

func (t *target) AsRecord() *Record {
	return &Record{Value: t.value}
}

func (t *target) String() string {
	return fmt.Sprintf("%s(%s)", t.GetRecordType(), t.GetRecordValue())
}

////////////////////////////////////////////////////////////////////////////////
// DNS Target Spec
////////////////////////////////////////////////////////////////////////////////

type TargetSpec interface {
	Targets() []Target
	RoutingPolicy() *RoutingPolicy
}

type targetSpec struct {
	targets       []Target
	routingPolicy *RoutingPolicy
}

func BaseTargetSpec(p TargetProvider) TargetSpec {
	spec := &targetSpec{
		targets:       p.Targets(),
		routingPolicy: p.RoutingPolicy(),
	}
	return spec
}

func (this *targetSpec) Targets() []Target {
	return this.targets
}

func (this *targetSpec) RoutingPolicy() *RoutingPolicy {
	return this.routingPolicy
}
