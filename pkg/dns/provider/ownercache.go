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

package provider

import (
	"sync"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"github.com/gardener/external-dns-management/pkg/dns/provider/statistic"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type ProviderTypeCounts map[string]int
type OwnerCounts map[string]ProviderTypeCounts

type OwnerObjectInfo struct {
	active bool
	id     string
}
type OwnerObjectInfos map[string]OwnerObjectInfo

type OwnerIdInfo struct {
	refcount    int
	entrycounts map[string]int
}
type OwnerIdInfos map[string]OwnerIdInfo

func (this OwnerIdInfos) Contains(id string) bool {
	_, ok := this[id]
	return ok
}
func (this OwnerIdInfos) KeySet() utils.StringSet {
	return utils.StringKeySet(this)
}

type OwnerCache struct {
	lock sync.RWMutex

	owners   OwnerObjectInfos
	ownerids OwnerIdInfos
}

func NewOwnerCache(config *Config) *OwnerCache {
	return &OwnerCache{
		owners:   OwnerObjectInfos{},
		ownerids: OwnerIdInfos{config.Ident: {refcount: 1, entrycounts: ProviderTypeCounts{}}},
	}
}

func (this *OwnerCache) IsResponsibleFor(id string) bool {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.ownerids.Contains(id)
}

func (this *OwnerCache) GetIds() utils.StringSet {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.ownerids.KeySet()
}

func (this *OwnerCache) UpdateCountsWith(statistic statistic.OwnerStatistic, types utils.StringSet) OwnerCounts {
	changed := OwnerCounts{}
	this.lock.Lock()
	defer this.lock.Unlock()
	for id, e := range this.ownerids {
		mod := false
		pts := statistic.Get(id)
		for t := range types {
			c := 0
			if v, ok := pts[t]; ok {
				c = v.Count()
			}
			mod = mod || this.checkCount(&e, t, c)
		}
		if mod {
			this.ownerids[id] = e
			for n, o := range this.owners {
				if o.id == id {
					changed[n] = e.entrycounts
				}
			}
		}
	}
	return changed
}

func (this *OwnerCache) checkCount(e *OwnerIdInfo, ptype string, count int) bool {
	if e.entrycounts[ptype] != count {
		e.entrycounts[ptype] = count
		return true
	}
	return false
}

func (this *OwnerCache) UpdateOwner(owner *dnsutils.DNSOwnerObject) (changeset utils.StringSet, activeset utils.StringSet) {
	return this.updateOwnerData(owner.ObjectName().String(), owner.GetOwnerId(), owner.IsActive(), owner.GetCounts())
}

func (this *OwnerCache) updateOwnerData(cachekey, id string, active bool, counts ProviderTypeCounts) (changeset utils.StringSet, activeset utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()

	changeset = utils.StringSet{}
	old, ok := this.owners[cachekey]
	if ok {
		if old.id == id && old.active == active {
			return changeset, this.ownerids.KeySet()
		}
		this.deactivate(cachekey, old, changeset)
	}
	this.activate(cachekey, id, active, changeset, counts)
	return changeset, this.ownerids.KeySet()
}

func (this *OwnerCache) DeleteOwner(key resources.ObjectKey) (changeset utils.StringSet, activeset utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()
	changeset = utils.StringSet{}
	cachekey := key.ObjectName().String()
	old, ok := this.owners[cachekey]
	if ok {
		this.deactivate(cachekey, old, changeset)
	}
	return changeset, this.ownerids.KeySet()
}

func (this *OwnerCache) deactivate(cachekey string, old OwnerObjectInfo, changeset utils.StringSet) {
	if old.active {
		e := this.ownerids[old.id]
		e.refcount--
		this.ownerids[old.id] = e
		if e.refcount == 0 {
			delete(this.ownerids, old.id)
			changeset.Add(old.id)
		}
	}
	delete(this.owners, cachekey)
}

func (this *OwnerCache) activate(cachekey string, id string, active bool, changeset utils.StringSet, counts ProviderTypeCounts) {
	if active {
		e, ok := this.ownerids[id]
		if !ok {
			e.entrycounts = counts
		}
		if e.entrycounts == nil {
			e.entrycounts = ProviderTypeCounts{}
		}
		e.refcount++
		this.ownerids[id] = e
		if e.refcount == 1 {
			changeset.Add(id)
		}
	}
	this.owners[cachekey] = OwnerObjectInfo{id: id, active: active}
}
