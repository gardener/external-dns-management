// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"sync"

	"github.com/gardener/controller-manager-library/pkg/resources"
)

type state struct {
	lock     sync.Mutex
	source   DNSSource
	feedback map[resources.ClusterObjectKey]DNSFeedback

	used map[resources.ClusterObjectKey]resources.ClusterObjectKeySet
	deps map[resources.ClusterObjectKey]resources.ClusterObjectKey
}

// NewState creates a new owner state.
func NewState() any {
	return &state{
		source:   nil,
		feedback: map[resources.ClusterObjectKey]DNSFeedback{},

		used: map[resources.ClusterObjectKey]resources.ClusterObjectKeySet{},
		deps: map[resources.ClusterObjectKey]resources.ClusterObjectKey{},
	}
}

func (this *state) CreateFeedbackForObject(obj resources.Object) DNSFeedback {
	fb := this.source.CreateDNSFeedback(obj)
	this.SetFeedback(obj.ClusterKey(), fb)
	return fb
}

func (this *state) GetFeedback(key resources.ClusterObjectKey) DNSFeedback {
	this.lock.Lock()
	defer this.lock.Unlock()

	return this.feedback[key]
}

func (this *state) SetFeedback(key resources.ClusterObjectKey, f DNSFeedback) {
	this.lock.Lock()
	defer this.lock.Unlock()

	this.feedback[key] = f
}

func (this *state) DeleteFeedback(key resources.ClusterObjectKey) {
	this.lock.Lock()
	defer this.lock.Unlock()

	delete(this.feedback, key)
}

func (this *state) SetDep(obj resources.ClusterObjectKey, dep *resources.ClusterObjectKey) {
	this.lock.Lock()
	defer this.lock.Unlock()

	old, ok := this.deps[obj]

	if ok && (dep == nil || *dep != old) {
		delete(this.deps, obj)
		set := this.used[old]
		if set != nil {
			set.Remove(obj)
			if len(set) == 0 {
				delete(this.used, old)
			}
		}
	}

	if dep != nil && (!ok || *dep != old) {
		this.deps[obj] = *dep
		set := this.used[*dep]
		if set == nil {
			set = resources.NewClusterObjectKeySet(obj)
			this.used[*dep] = set
		} else {
			set.Add(obj)
		}
	}
}

func (this *state) GetUsed(obj resources.ClusterObjectKey) resources.ClusterObjectKeySet {
	this.lock.Lock()
	defer this.lock.Unlock()

	set := this.used[obj]
	if set != nil {
		return resources.NewClusterObjectKeSetBySets(set)
	}
	return nil
}
