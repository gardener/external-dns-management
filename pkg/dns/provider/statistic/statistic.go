// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statistic

import (
	"github.com/gardener/controller-manager-library/pkg/resources"
)

type (
	WalkingState interface{}
	OwnerWalker  func(state WalkingState, owner, ptype string, pname resources.ObjectName, count int) WalkingState
)

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

type EntryStatistic struct {
	Providers ProviderTypeStatistic
}

func NewEntryStatistic() *EntryStatistic {
	return &EntryStatistic{ProviderTypeStatistic{}}
}
