// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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

func (this *References) NotifyHolder(pctx ProviderContext, ref resources.ClusterObjectKey) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	this.notifyHolder(pctx, ref)
}

func (this *References) notifyHolder(pctx ProviderContext, ref resources.ClusterObjectKey) {
	for h := range this.usages[ref] {
		_ = pctx.EnqueueKey(h)
		this.notifyHolder(pctx, h)
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
