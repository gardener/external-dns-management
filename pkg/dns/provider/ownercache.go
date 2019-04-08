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

	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type OwnerCache struct {
	lock sync.RWMutex

	ownerids utils.StringSet
	owners   map[resources.ObjectName]*dnsutils.DNSOwnerObject
	ownercnt map[string]int
}

func NewOwnerCache(config *Config) *OwnerCache {
	return &OwnerCache{
		ownerids: utils.NewStringSet(config.Ident),
		owners:   map[resources.ObjectName]*dnsutils.DNSOwnerObject{},
		ownercnt: map[string]int{config.Ident: 1},
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
	return this.ownerids.Copy()
}

func (this *OwnerCache) UpdateOwner(owner *dnsutils.DNSOwnerObject) (changed utils.StringSet, active utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()

	changed = utils.StringSet{}
	old := this.owners[owner.ObjectName()]
	if old != nil {
		if old.GetOwnerId() == owner.GetOwnerId() && old.IsActive() == owner.IsActive() {
			this.owners[owner.ObjectName()] = owner
			return changed, this.ownerids.Copy()
		}
		this.deactivate(old, changed)
	}
	this.activate(owner, changed)
	return changed, this.ownerids.Copy()
}

func (this *OwnerCache) DeleteOwner(key resources.ObjectKey) (changed utils.StringSet, active utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()
	changed = utils.StringSet{}
	this.deactivate(this.owners[key.ObjectName()], changed)
	return changed, this.ownerids.Copy()
}

func (this *OwnerCache) deactivate(old *dnsutils.DNSOwnerObject, changed utils.StringSet) {
	if old != nil {
		if old.IsActive() {
			cnt := this.ownercnt[old.GetOwnerId()]
			cnt--
			this.ownercnt[old.GetOwnerId()] = cnt
			if cnt == 0 {
				this.ownerids.Remove(old.GetOwnerId())
				changed.Add(old.GetOwnerId())
			}
		}
		delete(this.owners, old.ObjectName())
	}
}

func (this *OwnerCache) activate(new *dnsutils.DNSOwnerObject, changed utils.StringSet) {
	if new.IsActive() {
		id := new.GetOwnerId()
		cnt := this.ownercnt[id]
		cnt++
		this.ownercnt[id] = cnt
		this.ownerids.Add(id)
		if cnt == 1 {
			changed.Add(id)
		}
	}
	this.owners[new.ObjectName()] = new
}
