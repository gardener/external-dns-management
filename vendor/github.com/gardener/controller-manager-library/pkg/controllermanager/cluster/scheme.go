/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved.
 * This file is licensed under the Apache Software License, v. 2 except as noted
 * otherwise in the LICENSE file
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

package cluster

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
)

type SchemeCache interface {
	Cleanup(id string)
	Add(c Interface)
	Get(s *runtime.Scheme, id string) Interface
	WithScheme(c Interface, s *runtime.Scheme) (Interface, error)
}

type schemeCache struct {
	lock     sync.Mutex
	clusters map[*runtime.Scheme]clusters
}

var _ SchemeCache = (*schemeCache)(nil)

func NewSchemeCache() SchemeCache {
	return &schemeCache{clusters: map[*runtime.Scheme]clusters{}}
}

func (this schemeCache) Cleanup(id string) {
	this.lock.Lock()
	defer this.lock.Unlock()

	for _, m := range this.clusters {
		delete(m, id)
	}
}

func (this schemeCache) Add(c Interface) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.add(c)
}

func (this schemeCache) add(c Interface) {
	s := c.ResourceContext().Scheme()
	e := this.clusters[s]
	if e == nil {
		e = clusters{}
		this.clusters[s] = e
	}
	e[c.GetId()] = c
}

func (this schemeCache) Get(s *runtime.Scheme, id string) Interface {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.get(s, id)
}

func (this schemeCache) get(s *runtime.Scheme, id string) Interface {
	e := this.clusters[s]
	if e == nil {
		return nil
	}
	return e[id]
}

func (this schemeCache) WithScheme(c Interface, s *runtime.Scheme) (Interface, error) {
	this.lock.Lock()
	defer this.lock.Unlock()

	if c.ResourceContext().Scheme() == s {
		return c, nil
	}
	n := this.get(s, c.GetId())
	if n != nil {
		return n, nil
	}
	r, err := c.WithScheme(s)
	if err != nil {
		return nil, err
	}
	this.add(r)
	return r, nil
}
