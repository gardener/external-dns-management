// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"sync"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/statistic"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type ProviderCacheContext interface {
	GetContext() context.Context
	EnqueueKey(key resources.ClusterObjectKey) error

	Infof(msg string, args ...interface{})
}

type (
	ProviderTypeCounts map[string]int
	OwnerCounts        map[OwnerName]ProviderTypeCounts
)

type OwnerObjectInfo struct {
	active bool
	id     string
}

type OwnerName string

type OwnerObjectInfos map[OwnerName]OwnerObjectInfo

type OwnerIDInfo struct {
	refcount    int
	entrycounts map[string]int
}
type OwnerIDInfos map[string]OwnerIDInfo

func (this OwnerIDInfos) Contains(id string) bool {
	_, ok := this[id]
	return ok
}

func (this OwnerIDInfos) KeySet() utils.StringSet {
	return utils.StringKeySet(this)
}

type OwnerCache struct {
	lock sync.RWMutex
	ctx  ProviderCacheContext

	owners OwnerObjectInfos

	ownerids   OwnerIDInfos
	pendingids utils.StringSet
}

var _ dns.Ownership = &OwnerCache{}

func NewOwnerCache(ctx ProviderCacheContext, config *Config) *OwnerCache {
	this := &OwnerCache{
		ctx:        ctx,
		owners:     OwnerObjectInfos{},
		ownerids:   OwnerIDInfos{config.Ident: {refcount: 1, entrycounts: ProviderTypeCounts{}}},
		pendingids: utils.StringSet{},
	}
	return this
}

func (this *OwnerCache) IsResponsibleFor(id string) bool {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.ownerids.Contains(id)
}

func (this *OwnerCache) IsResponsiblePendingFor(id string) bool {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.pendingids.Contains(id)
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

func (this *OwnerCache) checkCount(e *OwnerIDInfo, ptype string, count int) bool {
	if e.entrycounts[ptype] != count {
		e.entrycounts[ptype] = count
		return true
	}
	return false
}

func (this *OwnerCache) UpdateOwner(owner *dnsutils.DNSOwnerObject) (changeset utils.StringSet, activeset utils.StringSet) {
	active := owner.IsActive()
	this.lock.Lock()
	defer this.lock.Unlock()
	return this._updateOwnerData(OwnerName(owner.GetName()), owner.GetOwnerId(), active, owner.GetCounts())
}

func (this *OwnerCache) updateOwnerData(cachekey OwnerName, id string, active bool, counts ProviderTypeCounts) (changeset utils.StringSet, activeset utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this._updateOwnerData(cachekey, id, active, counts)
}

func (this *OwnerCache) _updateOwnerData(cachekey OwnerName, id string, active bool, counts ProviderTypeCounts) (changeset utils.StringSet, activeset utils.StringSet) {
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

func (this *OwnerCache) DeleteOwner(key resources.ClusterObjectKey) (changeset utils.StringSet, activeset utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()
	changeset = utils.StringSet{}
	cachekey := OwnerName(key.Name())
	old, ok := this.owners[cachekey]
	if ok {
		this.deactivate(cachekey, old, changeset)
	}
	return changeset, this.ownerids.KeySet()
}

func (this *OwnerCache) deactivate(cachekey OwnerName, old OwnerObjectInfo, changeset utils.StringSet) {
	this.pendingids.Remove(old.id)
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

func (this *OwnerCache) activate(cachekey OwnerName, id string, active bool, changeset utils.StringSet, counts ProviderTypeCounts) {
	this.pendingids.Remove(id)
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

func (this *OwnerCache) SetPending(id string) {
	this.lock.Lock()
	defer this.lock.Unlock()
	if !this.ownerids.Contains(id) {
		this.pendingids.Add(id)
	}
}
