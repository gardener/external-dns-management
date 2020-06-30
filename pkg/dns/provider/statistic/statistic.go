/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package statistic

import (
	"github.com/gardener/controller-manager-library/pkg/resources"
)

type WalkingState interface{}
type OwnerWalker func(state WalkingState, owner, ptype string, pname resources.ObjectName, count int) WalkingState

type ProviderStatistic map[resources.ObjectName]int

func (this ProviderStatistic) Inc(name resources.ObjectName) {
	if name == nil {
		name = resources.NewObjectName("")
	}
	this[name]++
}

func (this ProviderStatistic) Count() int {
	sum := 0
	for _, e := range this {
		sum += e
	}
	return sum
}

func (this ProviderStatistic) Walk(state WalkingState, walker OwnerWalker, owner, ptype string) WalkingState {
	for pname, e := range this {
		state = walker(state, owner, ptype, pname, e)
	}
	return state
}

////////////////////////////////////////////////////////////////////////////////

type ProviderTypeStatistic map[string]ProviderStatistic

func (this ProviderTypeStatistic) Inc(ptype string, pname resources.ObjectName) {
	this.Assure(ptype).Inc(pname)
}

func (this ProviderTypeStatistic) Count() int {
	sum := 0
	for _, e := range this {
		sum += e.Count()
	}
	return sum
}

func (this ProviderTypeStatistic) Get(ptype string) ProviderStatistic {
	if ps := this[ptype]; ps != nil {
		return ps
	}
	return ProviderStatistic{}
}

func (this ProviderTypeStatistic) Assure(ptype string) ProviderStatistic {
	cur := this[ptype]
	if cur == nil {
		cur = ProviderStatistic{}
		this[ptype] = cur
	}
	return cur
}

func (this ProviderTypeStatistic) Walk(state WalkingState, walker OwnerWalker, owner string) WalkingState {
	for ptype, pts := range this {
		state = pts.Walk(state, walker, owner, ptype)
	}
	return state
}

////////////////////////////////////////////////////////////////////////////////

type OwnerStatistic map[string]ProviderTypeStatistic

func (this OwnerStatistic) Inc(owner, ptype string, pname resources.ObjectName) {
	this.Assure(owner).Inc(ptype, pname)
}

func (this OwnerStatistic) Count() int {
	sum := 0
	for _, e := range this {
		sum += e.Count()
	}
	return sum
}

func (this OwnerStatistic) Get(owner string) ProviderTypeStatistic {
	if pts := this[owner]; pts != nil {
		return pts
	}
	return ProviderTypeStatistic{}
}

func (this OwnerStatistic) Assure(owner string) ProviderTypeStatistic {
	cur := this[owner]
	if cur == nil {
		cur = ProviderTypeStatistic{}
		this[owner] = cur
	}
	return cur
}

func (this OwnerStatistic) Walk(state WalkingState, walker OwnerWalker) WalkingState {
	for o, os := range this {
		state = os.Walk(state, walker, o)
	}
	return state
}

////////////////////////////////////////////////////////////////////////////////

type EntryStatistic struct {
	Providers ProviderTypeStatistic
	Owners    OwnerStatistic
}

func NewEntryStatistic() *EntryStatistic {
	return &EntryStatistic{ProviderTypeStatistic{}, OwnerStatistic{}}
}
