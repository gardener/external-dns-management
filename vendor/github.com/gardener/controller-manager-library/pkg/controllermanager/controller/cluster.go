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

package controller

import (
	"fmt"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type clusterResourceInfo struct {
	resource    resources.Interface
	pools       []*pool
	namespace   string
	optionsFunc resources.TweakListOptionsFunc
}

func (this *clusterResourceInfo) List() ([]resources.Object, error) {
	opts := v1.ListOptions{}
	if this.optionsFunc != nil {
		this.optionsFunc(&opts)
	}
	if this.namespace != "" {
		return this.resource.Namespace(this.namespace).List(opts)
	}
	return this.resource.List(opts)
}

type ClusterHandler struct {
	logger.LogContext
	controller *controller
	cluster    cluster.Interface
	resources  map[ResourceKey]*clusterResourceInfo
}

func newClusterHandler(controller *controller, cluster cluster.Interface) *ClusterHandler {
	return &ClusterHandler{
		controller.NewContext("cluster", cluster.GetName()),
		controller,
		cluster,
		map[ResourceKey]*clusterResourceInfo{},
	}
}

func (c *ClusterHandler) whenReady() {
	c.controller.whenReady()
}

func (c *ClusterHandler) String() string {
	return c.cluster.GetName()
}

func (c *ClusterHandler) GetAliases() utils.StringSet {
	return c.controller.GetClusterAliases(c.cluster.GetName())
}

func (c *ClusterHandler) GetResource(resourceKey ResourceKey) (resources.Interface, error) {
	return c.cluster.GetResource(resourceKey.GroupKind())
}

func (c *ClusterHandler) register(resourceKey ResourceKey, namespace string, optionsFunc resources.TweakListOptionsFunc, usedpool *pool) error {
	i := c.resources[resourceKey]
	if i == nil {
		resource, err := c.cluster.GetResource(resourceKey.GroupKind())
		if err != nil {
			return err
		}

		i = &clusterResourceInfo{
			pools:       []*pool{usedpool},
			namespace:   namespace,
			optionsFunc: optionsFunc,
			resource:    resource,
		}
		c.resources[resourceKey] = i

		if err := resource.AddSelectedEventHandler(c.GetEventHandlerFuncs(), namespace, optionsFunc); err != nil {
			return err
		}
	} else {
		for _, p := range i.pools {
			if p == usedpool {
				return nil
			}
		}
		i.pools = append(i.pools, usedpool)
	}

	return nil
}

func (c *ClusterHandler) GetEventHandlerFuncs() resources.ResourceEventHandlerFuncs {
	return resources.ResourceEventHandlerFuncs{
		AddFunc:    c.objectAdd,
		UpdateFunc: c.objectUpdate,
		DeleteFunc: c.objectDelete,
	}
}

///////////////////////////////////////////////////////////////////////////////

func (c *ClusterHandler) EnqueueKey(key resources.ClusterObjectKey) error {
	// c.Infof("enqueue %s", obj.Description())
	gk := key.GroupKind()
	rk := NewResourceKey(gk.Group, gk.Kind)
	i := c.resources[rk]
	if i == nil {
		return fmt.Errorf("cluster %q: no resource info for %s", c, rk)
	}
	if i.pools == nil || len(i.pools) == 0 {
		return fmt.Errorf("cluster %q: no worker pool for type %s", c, rk)
	}
	for _, p := range i.pools {
		p.EnqueueKey(key)
	}
	return nil
}

func (c *ClusterHandler) enqueue(obj resources.Object, e func(p *pool, r resources.Object)) error {
	c.whenReady()
	// c.Infof("enqueue %s", obj.Description())
	i := c.resources[GetResourceKey(obj)]
	if i.pools == nil || len(i.pools) == 0 {
		return fmt.Errorf("no worker pool for type %s", obj.GroupKind())
	}
	for _, p := range i.pools {
		// p.Infof("enqueue %s", resources.ObjectrKey(obj))
		e(p, obj)
	}
	return nil
}

func enq(p *pool, obj resources.Object) {
	p.EnqueueObject(obj)
}

func (c *ClusterHandler) EnqueueObject(obj resources.Object) error {
	return c.enqueue(obj, enq)
}

func enqRateLimited(p *pool, obj resources.Object) {
	p.EnqueueObjectRateLimited(obj)
}
func (c *ClusterHandler) EnqueueObjectRateLimited(obj resources.Object) error {
	return c.enqueue(obj, enqRateLimited)
}

func (c *ClusterHandler) EnqueueObjectAfter(obj resources.Object, duration time.Duration) error {
	e := func(p *pool, obj resources.Object) {
		p.EnqueueObjectAfter(obj, duration)
	}
	return c.enqueue(obj, e)
}

///////////////////////////////////////////////////////////////////////////////

func (c *ClusterHandler) objectAdd(obj resources.Object) {
	c.Debugf("** GOT add event for %s", obj.Description())

	if c.controller.mustHandle(obj) {
		c.EnqueueObject(obj)
	}
}

func (c *ClusterHandler) objectUpdate(old, new resources.Object) {
	c.Debugf("** GOT update event for %s: %s", new.Description(), new.GetResourceVersion())
	if !c.controller.mustHandle(old) && !c.controller.mustHandle(new) {
		return
	}

	c.EnqueueObject(new)
}

func (c *ClusterHandler) objectDelete(obj resources.Object) {
	c.Debugf("** GOT delete event for %s: %s", obj.Description(), obj.GetResourceVersion())

	if c.controller.mustHandle(obj) {
		c.EnqueueObject(obj)
	}
}
