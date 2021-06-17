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
	"context"
	"sync"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns/provider/statistic"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type ProviderCacheContext interface {
	GetContext() context.Context
	EnqueueKey(key resources.ClusterObjectKey) error

	Infof(msg string, args ...interface{})
}

type ProviderTypeCounts map[string]int
type OwnerCounts map[OwnerName]ProviderTypeCounts

type OwnerObjectInfo struct {
	active bool
	id     string
}

type OwnerName string

type OwnerObjectInfos map[OwnerName]OwnerObjectInfo

type OwnerDNSActivation struct {
	Key     resources.ClusterObjectKey
	Current bool
	api.DNSActivation
}

func (this *OwnerDNSActivation) IsActive() bool {
	return dnsutils.CheckDNSActivation(this.Key.Cluster(), &this.DNSActivation)
}

type OwnerDNSActivations map[resources.ClusterObjectKey]*OwnerDNSActivation

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

	owners         OwnerObjectInfos
	dnsactivations OwnerDNSActivations

	ownerids   OwnerIDInfos
	pendingids utils.StringSet

	schedule *dnsutils.Schedule
}

func NewOwnerCache(ctx ProviderCacheContext, config *Config) *OwnerCache {
	this := &OwnerCache{
		ctx:            ctx,
		owners:         OwnerObjectInfos{},
		ownerids:       OwnerIDInfos{config.Ident: {refcount: 1, entrycounts: ProviderTypeCounts{}}},
		dnsactivations: OwnerDNSActivations{},
		pendingids:     utils.StringSet{},
	}
	this.schedule = dnsutils.NewSchedule(ctx.GetContext(), dnsutils.ScheduleExecutorFunction(this.expire))
	return this
}

func (this *OwnerCache) GetDNSActivations() OwnerDNSActivations {
	queries := OwnerDNSActivations{}
	this.lock.RLock()
	defer this.lock.RUnlock()
	for k, v := range this.dnsactivations {
		queries[k] = v
	}
	return queries
}

func (this *OwnerCache) TriggerDNSActivation(logger logger.LogContext, cntr controller.Interface) {
	for k, a := range this.GetDNSActivations() {
		if active := a.IsActive(); active != a.Current {
			logger.Infof("DNS activation changed for %s[%s] (%t)", k.ObjectName(), a.DNSName, active)
			cntr.EnqueueKey(k)
		}
	}
}

func (this *OwnerCache) expire(key dnsutils.ScheduleKey) {
	id := key.(resources.ClusterObjectKey)
	this.ctx.Infof("owner %s expired", id.Name())
	this.ctx.EnqueueKey(id)
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
	if activation := owner.GetDNSActivation(); activation != nil {
		this.dnsactivations[owner.ClusterKey()] = &OwnerDNSActivation{Key: owner.ClusterKey(), Current: active, DNSActivation: *activation}
	} else {
		delete(this.dnsactivations, owner.ClusterKey())
	}
	return this._updateOwnerData(OwnerName(owner.GetName()), owner.ClusterKey(), owner.GetOwnerId(), active, owner.GetCounts(), owner.ValidUntil())
}

func (this *OwnerCache) updateOwnerData(cachekey OwnerName, key dnsutils.ScheduleKey, id string, active bool, counts ProviderTypeCounts, valid *metav1.Time) (changeset utils.StringSet, activeset utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this._updateOwnerData(cachekey, key, id, active, counts, valid)
}

func (this *OwnerCache) _updateOwnerData(cachekey OwnerName, key dnsutils.ScheduleKey, id string, active bool, counts ProviderTypeCounts, valid *metav1.Time) (changeset utils.StringSet, activeset utils.StringSet) {
	changeset = utils.StringSet{}

	old, ok := this.owners[cachekey]
	if ok {
		if old.id == id && old.active == active {
			return changeset, this.ownerids.KeySet()
		}
		this.deactivate(cachekey, old, changeset)
	}
	if key != nil {
		if active && valid != nil {
			this.schedule.Schedule(key, (*valid).Time)
		} else {
			this.schedule.Delete(key)
		}
	}
	this.activate(cachekey, id, active, changeset, counts)
	return changeset, this.ownerids.KeySet()
}

func (this *OwnerCache) DeleteOwner(key resources.ObjectKey) (changeset utils.StringSet, activeset utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()
	changeset = utils.StringSet{}
	cachekey := OwnerName(key.Name())
	old, ok := this.owners[cachekey]
	if ok {
		this.schedule.Delete(key)
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
