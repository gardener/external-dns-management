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

type OwnerInfo struct {
	active bool
	id     string
}

type OwnerCache struct {
	lock sync.RWMutex

	ownerids utils.StringSet
	owners   map[string]OwnerInfo
	ownercnt map[string]int
}

func NewOwnerCache(config *Config) *OwnerCache {
	return &OwnerCache{
		ownerids: utils.NewStringSet(config.Ident),
		owners:   map[string]OwnerInfo{},
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

func (this *OwnerCache) UpdateOwner(owner *dnsutils.DNSOwnerObject) (changeset utils.StringSet, activeset utils.StringSet) {
	return this.UpdateOwnerData(owner.ObjectName().String(), owner.GetOwnerId(), owner.IsActive())
}

func (this *OwnerCache) UpdateOwnerData(name, id string, active bool) (changeset utils.StringSet, activeset utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()

	changeset = utils.StringSet{}
	old, ok := this.owners[name]
	if ok {
		if old.id == id && old.active == active {
			return changeset, this.ownerids.Copy()
		}
		this.deactivate(name, old, changeset)
	}
	this.activate(name, id, active, changeset)
	return changeset, this.ownerids.Copy()
}

func (this *OwnerCache) DeleteOwner(key resources.ObjectKey) (changeset utils.StringSet, activeset utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()
	changeset = utils.StringSet{}
	name := key.ObjectName().String()
	old, ok := this.owners[name]
	if ok {
		this.deactivate(name, old, changeset)
	}
	return changeset, this.ownerids.Copy()
}

func (this *OwnerCache) deactivate(name string, old OwnerInfo, changeset utils.StringSet) {
	if old.active {
		cnt := this.ownercnt[old.id]
		cnt--
		this.ownercnt[old.id] = cnt
		if cnt == 0 {
			this.ownerids.Remove(old.id)
			changeset.Add(old.id)
		}
	}
	delete(this.owners, name)
}

func (this *OwnerCache) activate(name string, id string, active bool, changeset utils.StringSet) {
	if active {
		cnt := this.ownercnt[id]
		cnt++
		this.ownercnt[id] = cnt
		this.ownerids.Add(id)
		if cnt == 1 {
			changeset.Add(id)
		}
	}
	this.owners[name] = OwnerInfo{id: id, active: active}
}
