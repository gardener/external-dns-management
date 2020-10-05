/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewGroupKindFilter(gk schema.GroupKind) KeyFilter {
	return func(key ClusterObjectKey) bool {
		return key.GroupKind() == gk
	}
}

type entry struct {
	object Object
	count  int
}

type SlaveCache struct {
	migration ClusterIdMigration
	cache     SubObjectCache
}

func NewSlaveCache(migration ...ClusterIdMigration) *SlaveCache {
	var mig ClusterIdMigration
	if len(migration) > 0 {
		mig = migration[0]
	}
	return &SlaveCache{mig, *NewSubObjectCache(func(o Object) ClusterObjectKeySet { return o.GetOwners() })}
}

func (this *SlaveCache) AddOwnerFilter(filters ...KeyFilter) *SlaveCache {
	this.cache.AddOwnerFilter(filters...)
	return this
}

func (this *SlaveCache) AddSlaveFilter(filters ...ObjectFilter) *SlaveCache {
	this.cache.AddSubObjectFilter(filters...)
	return this
}

func (this *SlaveCache) Size() int {
	return this.cache.Size()
}

func (this *SlaveCache) SlaveCount() int {
	return this.cache.SubObjectCount()
}

func (this *SlaveCache) Setup(slaves []Object) {
	if this.migration != nil {
		for _, s := range slaves {
			err := MigrateOwnerClusterIds(s, this.migration)
			if err != nil {
				panic(fmt.Errorf("owner cluster id migration failed for %s: %s", s.ClusterKey(), err))
			}
		}
	}
	this.cache.Setup(slaves)
}

func (this *SlaveCache) GetSlave(key ClusterObjectKey) Object {
	return this.cache.GetSubObject(key)
}

func (this *SlaveCache) GetOwners(kinds ...schema.GroupKind) ClusterObjectKeySet {
	return this.cache.GetAllOwners(kinds...)
}

func (this *SlaveCache) GetOwnersFor(key ClusterObjectKey, kinds ...schema.GroupKind) ClusterObjectKeySet {
	o := this.GetSlave(key)
	if o == nil {
		return ClusterObjectKeySet{}
	}
	return o.GetOwners(kinds...)
}

func (this *SlaveCache) DeleteSlave(key ClusterObjectKey) {
	this.cache.DeleteSubObject(key)
}

func (this *SlaveCache) DeleteOwner(key ClusterObjectKey) {
	this.cache.DeleteOwner(key)
}

func (this *SlaveCache) RenewSlaveObject(obj Object) bool {
	return this.cache.RenewSubObject(obj)
}

func (this *SlaveCache) UpdateSlave(obj Object) error {
	return this.cache.UpdateSubObject(obj)
}

// Get is replaced by GetByOwner
// Deprecated: Please use GetByOwner
func (this *SlaveCache) Get(obj Object) []Object {
	return this.cache.GetByOwner(obj)
}
func (this *SlaveCache) GetByOwner(obj Object) []Object {
	return this.cache.GetByOwner(obj)
}

// GetByKey is replaced by GetByOwnerKey
// Deprecated: Please use GetByOwnerKey
func (this *SlaveCache) GetByKey(key ClusterObjectKey) []Object {
	return this.cache.GetByOwnerKey(key)
}
func (this *SlaveCache) GetByOwnerKey(key ClusterObjectKey) []Object {
	return this.cache.GetByOwnerKey(key)
}

func (this *SlaveCache) AddSlave(obj Object, slave Object) error {
	if slave.AddOwner(obj) {
		return this.cache.UpdateSubObject(slave)
	}
	return nil
}

func (this *SlaveCache) CreateSlave(obj Object, slave Object) error {
	slave.AddOwner(obj)
	return this.cache.CreateSubObject(slave)
}

func (this *SlaveCache) CreateOrModifySlave(obj Object, slave Object, mod Modifier) (bool, error) {
	slave.AddOwner(obj)
	return this.cache.CreateOrModifySubObject(slave, mod)
}

func (this *SlaveCache) Remove(obj Object, slave Object) bool {
	mod := slave.RemoveOwner(obj)
	if mod {
		this.cache.UpdateSubObject(slave)
	}
	return mod
}
