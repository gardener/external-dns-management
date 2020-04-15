/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
 *
 */

package provider

import (
	"sync"

	"github.com/gardener/controller-manager-library/pkg/resources"
)

type References struct {
	lock sync.RWMutex

	refs   map[resources.ClusterObjectKey]resources.ClusterObjectKey
	usages map[resources.ClusterObjectKey]resources.ClusterObjectKeySet
}

func NewReferenceCache() *References {
	return &References{
		refs:   map[resources.ClusterObjectKey]resources.ClusterObjectKey{},
		usages: map[resources.ClusterObjectKey]resources.ClusterObjectKeySet{},
	}
}

func (this *References) AddRef(holder resources.ClusterObjectKey, ref resources.ClusterObjectKey) {
	this.lock.Lock()
	defer this.lock.Unlock()

	old, ok := this.refs[holder]
	if ok && old == ref {
		return
	}
	set := this.usages[ref]
	if set == nil {
		set = resources.ClusterObjectKeySet{}
		this.usages[ref] = set
	}
	set.Add(holder)
	this.del(holder)
}

func (this *References) DelRef(holder resources.ClusterObjectKey) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.del(holder)
}

func (this *References) NotifyHolder(ctx Context, ref resources.ClusterObjectKey) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	this.notifyHolder(ctx, ref)
}

func (this *References) notifyHolder(ctx Context, ref resources.ClusterObjectKey) {
	for h := range this.usages[ref] {
		ctx.EnqueueKey(h)
		this.notifyHolder(ctx, h)
	}
}

func (this *References) del(holder resources.ClusterObjectKey) {
	old, ok := this.refs[holder]
	if !ok {
		return
	}
	delete(this.refs, holder)

	set := this.usages[old]
	if set == nil {
		return
	}
	set.Remove(holder)
	if len(set) != 0 {
		return
	}
	delete(this.usages, old)
}
