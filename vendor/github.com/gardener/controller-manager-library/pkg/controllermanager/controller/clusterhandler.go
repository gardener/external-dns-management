/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"fmt"
	"reflect"
	"sync"
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
	cache      sync.Map
}

func newClusterHandler(controller *controller, cluster cluster.Interface) *ClusterHandler {
	return &ClusterHandler{
		LogContext: controller.NewContext("cluster", cluster.GetName()),
		controller: controller,
		cluster:    cluster,
		resources:  map[ResourceKey]*clusterResourceInfo{},
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
		if i.namespace != namespace {
			return fmt.Errorf("watch namespace mismatch for resource %s (%q != %q)", resourceKey, i.namespace, namespace)
		}
		if (i.optionsFunc == nil) != (optionsFunc == nil) {
			return fmt.Errorf("watch options mismatch for resource %s", resourceKey)
		}
		if optionsFunc != nil {
			opts1 := &v1.ListOptions{}
			opts2 := &v1.ListOptions{}
			i.optionsFunc(opts1)
			optionsFunc(opts2)
			if !reflect.DeepEqual(opts1, opts2) {
				return fmt.Errorf("watch options mismatch for resource %s (%+v != %+v)", resourceKey, opts1, opts2)
			}
		}
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

func (c *ClusterHandler) GetObject(key resources.ClusterObjectKey) (resources.Object, error) {
	o, ok := c.cache.Load(key.ObjectKey())
	if o == nil || !ok {
		return nil, nil
	}
	return o.(resources.Object), nil
}

func (c *ClusterHandler) objectAdd(obj resources.Object) {
	c.Debugf("** GOT add event for %s", obj.Description())

	if c.controller.mustHandle(obj) {
		c.cache.Store(obj.Key(), obj)
		c.EnqueueObject(obj)
	}
}

func (c *ClusterHandler) objectUpdate(old, new resources.Object) {
	c.Debugf("** GOT update event for %s: %s", new.Description(), new.GetResourceVersion())
	if !c.controller.mustHandle(old) && !c.controller.mustHandle(new) {
		return
	}
	c.cache.Store(new.Key(), new)
	c.EnqueueObject(new)
}

func (c *ClusterHandler) objectDelete(obj resources.Object) {
	c.Debugf("** GOT delete event for %s: %s", obj.Description(), obj.GetResourceVersion())

	if c.controller.mustHandle(obj) {
		c.cache.Delete(obj.Key())
		c.EnqueueObject(obj)
	}
}
