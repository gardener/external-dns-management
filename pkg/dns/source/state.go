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
 * limitations under the License.
 *
 */

package source

import (
	"sync"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

type state struct {
	lock     sync.Mutex
	source   DNSSource
	feedback map[resources.ClusterObjectKey]DNSFeedback

	used map[resources.ClusterObjectKey]resources.ClusterObjectKeySet
	deps map[resources.ClusterObjectKey]resources.ClusterObjectKey
}

func NewState() interface{} {
	return &state{
		source:   nil,
		feedback: map[resources.ClusterObjectKey]DNSFeedback{},

		used: map[resources.ClusterObjectKey]resources.ClusterObjectKeySet{},
		deps: map[resources.ClusterObjectKey]resources.ClusterObjectKey{},
	}
}

func (this *state) GetFeedbackForObject(obj resources.Object) DNSFeedback {
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

func (this *state) EnqueueUsers(obj resources.ClusterObjectKey, c controller.Interface) {
	this.lock.Lock()
	defer this.lock.Unlock()

	set := this.used[obj]
	if set != nil {
		for u := range this.used[obj] {
			c.EnqueueKey(u)
		}
	}
}
