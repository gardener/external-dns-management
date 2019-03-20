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

package reconcilers

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

////////////////////////////////////////////////////////////////////////////////
// SlaveAccess to be used as common nested base for all reconcilers
// requiring slave access
////////////////////////////////////////////////////////////////////////////////

type UsageAccess struct {
	controller.Interface
	reconcile.DefaultReconciler
	used             resources.UsedExtractor
	name             string
	usages           *resources.UsageCache
	master_resources *_resources
	spec             UsageAccessSpec
}

type UsedExtractorFactory func(controller.Interface) resources.UsedExtractor

type UsageAccessSpec struct {
	Name             string
	MasterResources  Resources
	Extractor        resources.UsedExtractor
	ExtractorFactory UsedExtractorFactory
}

func NewUsageAccessBySpec(c controller.Interface, spec UsageAccessSpec) *UsageAccess {
	used := spec.Extractor
	if used == nil {
		used = spec.ExtractorFactory(c)
	}
	return &UsageAccess{
		Interface:        c,
		name:             spec.Name,
		used:             used,
		master_resources: newResources(c, spec.MasterResources),
		spec:             spec,
	}
}

func (this *UsageAccess) Key() interface{} {
	return accesskey{name: this.name, masters: this.master_resources.String(), slaves: ""}
}

func (this *UsageAccess) Setup() {
	this.usages = this.GetOrCreateSharedValue(this.Key(), this.setupUsageCache).(*resources.UsageCache)
	if this.usages == nil {
		panic("no usages created")
	}
}

func (this *UsageAccess) setupUsageCache() interface{} {
	cache := resources.NewUsageCache(this.used)

	this.Infof("setup %s usage cache", this.name)
	for _, r := range this.master_resources.resources {
		list, _ := r.ListCached(labels.Everything())
		cache.Setup(list)
	}
	this.Infof("found %d %s(s) for %d objects", cache.UsedCount(), this.name, cache.Size())
	return cache
}

func (this *UsageAccess) MasterResoures() []resources.Interface {
	return this.master_resources.resources
}

func (this *UsageAccess) LookupUsages(key resources.ClusterObjectKey, kinds ...schema.GroupKind) resources.ClusterObjectKeySet {

	if len(kinds) == 0 {
		return this.usages.GetUsages(key).Copy()
	}
	found := resources.ClusterObjectKeySet{}
	for o := range this.usages.GetUsages(key) {
		for _, k := range kinds {
			if o.GroupKind() == k {
				found.Add(o)
			}
		}
	}
	return found
}

func (this *UsageAccess) Usages() *resources.UsageCache {
	return this.usages
}

func (this *UsageAccess) RenewOwner(obj resources.Object) bool {
	return this.usages.RenewOwner(obj)
}

func (this *UsageAccess) DeleteOwner(key resources.ClusterObjectKey) {
	this.usages.DeleteOwner(key)
}

func (this *UsageAccess) GetOwnersFor(key resources.ClusterObjectKey, all_clusters bool, kinds ...schema.GroupKind) resources.ClusterObjectKeySet {
	set := this.usages.GetOwnersFor(key, kinds...)
	if all_clusters {
		return set
	}
	return filterKeysByClusters(set, this.master_resources.clusters)
}

func (this *UsageAccess) GetOwners(all_clusters bool, kinds ...schema.GroupKind) resources.ClusterObjectKeySet {
	set := this.usages.GetOwners()

	if all_clusters {
		return set
	}
	return filterKeysByClusters(set, this.master_resources.clusters, kinds...)
}

func (this *UsageAccess) GetUsed(all_clusters bool, kinds ...schema.GroupKind) resources.ClusterObjectKeySet {
	set := this.usages.GetUsed()

	if all_clusters {
		return set
	}
	return filterKeysByClusters(set, this.master_resources.clusters, kinds...)
}

////////////////////////////////////////////////////////////////////////////////
// UsageReconciler used as Reconciler registered for watching source or
// target objects of a usage relation
//  nested reconcilers can cast the controller interface to *UsageReconciler
////////////////////////////////////////////////////////////////////////////////

func UsageReconcilerTypeBySpec(reconciler controller.ReconcilerType, spec UsageAccessSpec) controller.ReconcilerType {
	return func(c controller.Interface) (reconcile.Interface, error) {
		return NewUsageReconcilerBySpec(c, reconciler, spec)
	}
}

func NewUsageReconcilerBySpec(c controller.Interface, reconciler controller.ReconcilerType, spec UsageAccessSpec) (*UsageReconciler, error) {
	r := &UsageReconciler{
		UsageAccess: NewUsageAccessBySpec(c, spec),
	}
	nested, err := NewNestedReconciler(reconciler, r)
	if err != nil {
		return nil, err
	}
	r.NestedReconciler = nested
	return r, nil
}

type UsageReconciler struct {
	*NestedReconciler
	*UsageAccess
}

var _ reconcile.Interface = &UsageReconciler{}

func (this *UsageReconciler) Setup() {
	this.UsageAccess.Setup()
	this.NestedReconciler.Setup()
}

func (this *UsageReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	if this.master_resources.Contains(obj.GroupKind()) {
		logger.Infof("reconcile owner %s", obj.ClusterKey())
		this.usages.RenewOwner(obj)
	} else {
		logger.Infof("reconcile used %s", obj.ClusterKey())
		this.requeueMasters(logger, this.GetOwnersFor(obj.ClusterKey(), false))
	}
	return this.NestedReconciler.Reconcile(logger, obj)
}

func (this *UsageReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	if this.master_resources.Contains(key.GroupKind()) {
		logger.Infof("deleted owner %s", key)
		this.usages.DeleteOwner(key)
	} else {
		logger.Infof("deleted used %s", key)
		this.requeueMasters(logger, this.GetOwnersFor(key, false))
	}
	return this.NestedReconciler.Deleted(logger, key)
}

func (this *UsageReconciler) requeueMasters(logger logger.LogContext, masters resources.ClusterObjectKeySet) {
	for key := range masters {
		m, err := this.GetObject(key)
		if err == nil || errors.IsNotFound(err) {
			if m.IsDeleting() {
				logger.Infof("skipping requeue of deleting master %s", key)
				continue
			}
		}
		logger.Infof("requeue master %s", key)
		this.EnqueueKey(key)
	}
}
