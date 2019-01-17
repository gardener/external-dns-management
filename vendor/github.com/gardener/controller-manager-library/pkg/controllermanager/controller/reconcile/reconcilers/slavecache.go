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

type SlaveResources func(c controller.Interface) []resources.Interface

////////////////////////////////////////////////////////////////////////////////
// SlaveAccess to be used as common nested base for all reconcilers
// requiring slave access
////////////////////////////////////////////////////////////////////////////////

type SlaveAccess struct {
	controller.Interface
	reconcile.DefaultReconciler
	name            string
	slaves          *resources.SlaveCache
	slave_resources []resources.Interface
	kinds           []schema.GroupKind
	masters         []schema.GroupKind
}

func NewSlaveAccess(c controller.Interface, name string, res SlaveResources, masters ...schema.GroupKind) *SlaveAccess {
	var kinds []schema.GroupKind
	array := res(c)
	for _, r := range array {
		kinds = append(kinds, r.GroupKind())
	}
	return &SlaveAccess{
		Interface:       c,
		name:            name,
		slave_resources: array,
		kinds:           kinds,
		masters:         masters,
	}
}

type key struct {
	names string
}

func (this *SlaveAccess) Setup() {
	key := key{}
	for _, r := range this.slave_resources {
		gk := r.GroupKind()
		key.names = key.names + "|" + (&gk).String()
	}
	this.slaves = this.GetOrCreateSharedValue(key, this.setupSlaveCache).(*resources.SlaveCache)
}

func (this *SlaveAccess) setupSlaveCache() interface{} {
	cache := resources.NewSlaveCache()

	this.Infof("setup %s owner cache", this.name)
	for _, r := range this.slave_resources {
		list, _ := r.ListCached(labels.Everything())
		cache.Setup(list)
	}
	this.Infof("found %d %s(s) for %d owners", cache.SlaveCount(), this.name, cache.Size())
	return cache
}

func (this *SlaveAccess) SlaveResoures() []resources.Interface {
	return this.slave_resources
}

func (this *SlaveAccess) CreateSlave(obj resources.Object, slave resources.Object) error {
	return this.slaves.CreateSlave(obj, slave)
}

func (this *SlaveAccess) UpdateSlave(slave resources.Object) error {
	return this.slaves.UpdateSlave(slave)
}

func (this *SlaveAccess) AddSlave(obj resources.Object, slave resources.Object) error {
	return this.slaves.AddSlave(obj, slave)
}

func (this *SlaveAccess) LookupSlaves(key resources.ClusterObjectKey, kinds ...schema.GroupKind) []resources.Object {
	found := []resources.Object{}
	if len(kinds) == 0 {
		kinds = this.kinds
	}
	for _, o := range this.slaves.GetByKey(key) {
		for _, k := range kinds {
			if o.GroupKind() == k {
				found = append(found, o)
			}
		}
	}
	return found
}

func (this *SlaveAccess) AssertSingleSlave(logger logger.LogContext, key resources.ClusterObjectKey, slaves []resources.Object, match resources.ObjectMatcher) resources.Object {
	var found resources.Object
	for _, o := range slaves {
		if match == nil || match(o) {
			if found != nil {
				if o.GetCreationTimestamp().Time.Before(found.GetCreationTimestamp().Time) {
				} else {
					found, o = o, found
				}
				err := o.Delete()
				if err != nil {
					logger.Warnf("cleanup of obsolete %s %s for %s failed %s", this.name, o.ObjectName(), key.ObjectName(), err)
				} else {
					logger.Infof("cleanup of obsolete %s %s for %s", this.name, o.ObjectName(), key.ObjectName())
				}
			} else {
				found = o
			}
		}
	}
	return found
}

func (this *SlaveAccess) Slaves() *resources.SlaveCache {
	return this.slaves
}

func (this *SlaveAccess) GetMasters(key resources.ClusterObjectKey, kinds ...schema.GroupKind) resources.ClusterObjectKeySet {
	if len(kinds) > 0 {
		return this.slaves.GetOwners(key, kinds...)
	} else {
		return this.slaves.GetOwners(key, this.masters...)
	}
}

////////////////////////////////////////////////////////////////////////////////
// SlaveReconciler used as Reconciler registered for watching slave object
//  nested reconcilers can cast the controller interface to *SlaveReconciler
////////////////////////////////////////////////////////////////////////////////

func SlaveReconcilerType(name string, slaveResources SlaveResources, reconciler controller.ReconcilerType, masters ...schema.GroupKind) controller.ReconcilerType {
	return func(c controller.Interface) (reconcile.Interface, error) {
		return NewSlaveReconciler(c, name, slaveResources, reconciler, masters...)
	}
}

func NewSlaveReconciler(c controller.Interface, name string, slaveResources SlaveResources, reconciler controller.ReconcilerType, masters ...schema.GroupKind) (*SlaveReconciler, error) {
	r := &SlaveReconciler{
		SlaveAccess: NewSlaveAccess(c, name, slaveResources, masters...),
	}
	nested, err := NewNestedReconciler(reconciler, r)
	if err != nil {
		return nil, err
	}
	r.NestedReconciler = nested
	return r, nil
}

type SlaveReconciler struct {
	*NestedReconciler
	*SlaveAccess
}

var _ reconcile.Interface = &SlaveReconciler{}

func (this *SlaveReconciler) Setup() {
	this.SlaveAccess.Setup()
	this.NestedReconciler.Setup()
}

func (this *SlaveReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	this.slaves.RenewSlaveObject(obj)
	logger.Infof("reconcile slave %s", obj.ClusterKey())
	this.requeueMasters(logger, this.GetMasters(obj.ClusterKey()))
	return this.NestedReconciler.Reconcile(logger, obj)
}

func (this *SlaveReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	masters := this.GetMasters(key)
	this.slaves.DeleteSlave(key)
	this.requeueMasters(logger, masters)
	return this.NestedReconciler.Deleted(logger, key)
}

func (this *SlaveReconciler) requeueMasters(logger logger.LogContext, masters resources.ClusterObjectKeySet) {
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
