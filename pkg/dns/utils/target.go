/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package utils

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/utils"

	"github.com/gardener/external-dns-management/pkg/dns"
)

////////////////////////////////////////////////////////////////////////////////
// DNS Target
////////////////////////////////////////////////////////////////////////////////

type Targets []Target

func (this Targets) Has(target Target) bool {
	for _, t := range this {
		if t.GetRecordType() == target.GetRecordType() &&
			t.GetHostName() == target.GetHostName() {
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
}

type target struct {
	rtype string
	host  string
	ttl   int64
}

func NewText(t string, ttl int64) Target {
	return NewTarget(dns.RS_TXT, fmt.Sprintf("%q", t), ttl)
}

func NewTarget(ty string, ta string, ttl int64) Target {
	return &target{rtype: ty, host: ta, ttl: ttl}
}

func (t *target) GetTTL() int64         { return t.ttl }
func (t *target) GetHostName() string   { return t.host }
func (t *target) GetRecordType() string { return t.rtype }

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
	Responsible(set *dns.DNSSet, owners utils.StringSet) bool
}

type targetSpec struct {
	kind    string
	ownerId string
	targets []Target
}

func BaseTargetSpec(entry DNSSpecification, p TargetProvider) TargetSpec {
	spec := &targetSpec{
		kind:    entry.GroupKind().Kind,
		ownerId: p.OwnerId(),
		targets: p.Targets(),
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

func (this *targetSpec) Responsible(set *dns.DNSSet, owners utils.StringSet) bool {
	return !set.IsForeign(owners)
}
