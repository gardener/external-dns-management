/*
 * Copyright 2020 Mandelsoft. All rights reserved.
 *  This file is licensed under the Apache Software License, v. 2 except as noted
 *  otherwise in the LICENSE file
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
 */

package reconcilers

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

var usersKey = ctxutil.SimpleKey("users")

// GetSharedSimpleUsageCache returns an instance of a usage cache unique for
// the complete controller manager
func GetSharedSimpleUsageCache(controller controller.Interface) *SimpleUsageCache {
	return controller.GetEnvironment().GetOrCreateSharedValue(usersKey, func() interface{} {
		return NewSimpleUsageCache()
	}).(*SimpleUsageCache)
}

type SimpleUsageCache struct {
	lock        sync.RWMutex
	users       map[resources.ClusterObjectKey]resources.ClusterObjectKeySet
	uses        map[resources.ClusterObjectKey]resources.ClusterObjectKeySet
	reconcilers map[string]resources.GroupKindSet
}

func NewSimpleUsageCache() *SimpleUsageCache {
	return &SimpleUsageCache{
		users:       map[resources.ClusterObjectKey]resources.ClusterObjectKeySet{},
		uses:        map[resources.ClusterObjectKey]resources.ClusterObjectKeySet{},
		reconcilers: map[string]resources.GroupKindSet{},
	}
}

// reconcilerFor is used to assure that only one reconciler in one controller
// handles the usage reconcilations. The usage cache is hold at controller
// extensio level and is shared among all controllers of a controller manager.
func (this *SimpleUsageCache) reconcilerFor(cluster cluster.Interface, gks ...schema.GroupKind) resources.GroupKindSet {
	responsible := resources.GroupKindSet{}

	this.lock.Lock()
	defer this.lock.Unlock()
	actual := this.reconcilers[cluster.GetId()]
	if actual == nil {
		actual = resources.GroupKindSet{}
		this.reconcilers[cluster.GetId()] = actual
	}
	for _, gk := range gks {
		if !actual.Contains(gk) {
			responsible.Add(gk)
			actual.Add(gk)
		}
	}
	return responsible
}

func (this *SimpleUsageCache) GetUsersFor(name resources.ClusterObjectKey) resources.ClusterObjectKeySet {
	this.lock.RLock()
	defer this.lock.RUnlock()

	set := this.users[name]
	if set == nil {
		return nil
	}
	return set.Copy()
}

func (this *SimpleUsageCache) GetUsesFor(name resources.ClusterObjectKey) resources.ClusterObjectKeySet {
	this.lock.RLock()
	defer this.lock.RUnlock()

	set := this.uses[name]
	if set == nil {
		return nil
	}
	return set.Copy()
}

func (this *SimpleUsageCache) SetUsesFor(user resources.ClusterObjectKey, used *resources.ClusterObjectKey) {
	if used == nil {
		this.UpdateUsesFor(user, nil)
	} else {
		this.UpdateUsesFor(user, resources.NewClusterObjectKeySet(*used))
	}
}

func (this *SimpleUsageCache) UpdateUsesFor(user resources.ClusterObjectKey, uses resources.ClusterObjectKeySet) {
	this.lock.Lock()
	defer this.lock.Unlock()

	var add, del resources.ClusterObjectKeySet
	old := this.uses[user]
	if old != nil {
		add, del = old.DiffFrom(uses)
		this.cleanup(user, del)
	} else {
		add = uses
	}
	if len(this.uses) > 0 {
		this.uses[user] = uses
	} else {
		delete(this.uses, user)
	}
	if len(add) > 0 {
		for used := range uses {
			this.addUseFor(user, used)
		}
	}
}

func (this *SimpleUsageCache) AddUseFor(user resources.ClusterObjectKey, used resources.ClusterObjectKey) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.addUseFor(user, used)
}

func (this *SimpleUsageCache) addUseFor(user resources.ClusterObjectKey, used resources.ClusterObjectKey) {
	set := this.users[used]
	if set == nil {
		set = resources.NewClusterObjectKeySet()
		this.users[used] = set
	}
	set.Add(user)

	set = this.uses[user]
	if set == nil {
		set = resources.NewClusterObjectKeySet()
		this.uses[user] = set
	}
	set.Add(used)
}

func (this *SimpleUsageCache) RemoveUseFor(user resources.ClusterObjectKey, used resources.ClusterObjectKey) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.removeUseFor(user, used)
}

func (this *SimpleUsageCache) removeUseFor(user resources.ClusterObjectKey, used resources.ClusterObjectKey) {
	set := this.users[used]
	if set != nil {
		set.Remove(user)
		if len(set) == 0 {
			delete(this.users, used)
		}
	}

	set = this.uses[user]
	if set != nil {
		set.Remove(used)
		if len(set) == 0 {
			delete(this.uses, user)
		}
	}
}

func (this *SimpleUsageCache) cleanup(user resources.ClusterObjectKey, uses resources.ClusterObjectKeySet) {
	if len(uses) > 0 {
		for l := range uses {
			this.removeUseFor(user, l)
		}
	}
}

////////////////////////////////////////////////////////////////////////////////

type usageReconciler struct {
	ReconcilerSupport
	cache       *SimpleUsageCache
	clusterId   string
	responsible resources.GroupKindSet
}

var _ reconcile.Interface = &usageReconciler{}
var _ reconcile.ReconcilationRejection = &usageReconciler{}

func (this *usageReconciler) RejectResourceReconcilation(cluster cluster.Interface, gk schema.GroupKind) bool {
	if cluster == nil || this.clusterId != cluster.GetId() {
		return true
	}
	return !this.responsible.Contains(gk)
}

func (this *usageReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	users := this.cache.GetUsersFor(obj.ClusterKey())
	if len(users) > 0 {
		logger.Infof("%s updated -> trigger using objects", obj.ClusterKey())
		for n := range users {
			this.controller.GetEnvironment().EnqueueKey(n)
		}
	}
	return reconcile.Succeeded(logger)
}

func (this *usageReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	users := this.cache.GetUsersFor(key)
	if len(users) > 0 {
		logger.Infof("%s deleted -> trigger using objects", key)
		for n := range users {
			this.controller.GetEnvironment().EnqueueKey(n)
		}
	}
	return reconcile.Succeeded(logger)
}

////////////////////////////////////////////////////////////////////////////////

func UsageReconcilerForGKs(name string, cluster string, gks ...schema.GroupKind) controller.ConfigurationModifier {
	return func(c controller.Configuration) controller.Configuration {
		if c.Definition().Reconcilers()[name] == nil {
			c = c.Reconciler(CreateSimpleUsageReconcilerTypeFor(cluster, gks...), name)
		}
		return c.Cluster(cluster).ReconcilerWatchesByGK(name, gks...)
	}
}

func CreateSimpleUsageReconcilerTypeFor(clusterName string, gks ...schema.GroupKind) controller.ReconcilerType {
	return func(controller controller.Interface) (reconcile.Interface, error) {
		cache := GetSharedSimpleUsageCache(controller)
		cluster := controller.GetCluster(clusterName)
		if cluster == nil {
			return nil, fmt.Errorf("cluster %s not found", clusterName)
		}
		this := &usageReconciler{
			ReconcilerSupport: NewReconcilerSupport(controller),
			cache:             cache,
			clusterId:         cluster.GetId(),
			responsible:       cache.reconcilerFor(cluster, gks...),
		}
		return this, nil
	}
}
