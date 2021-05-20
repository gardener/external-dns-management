/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources/minimal"
)

func convert(resource Interface, funcs *ResourceEventHandlerFuncs) *cache.ResourceEventHandlerFuncs {
	return &cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if funcs.AddFunc == nil {
				return
			}
			o, err := resource.Wrap(obj.(ObjectData))
			if err == nil {
				funcs.AddFunc(o)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if funcs.DeleteFunc == nil {
				return
			}
			data, ok := obj.(ObjectData)
			if !ok {
				stale, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					logger.Errorf("informer %q reported unknown object to be deleted (%T)", resource.Name(), obj)
					return
				}
				if stale.Obj == nil {
					logger.Errorf("informer %q reported no stale object to be deleted", resource.Name())
					return
				}
				data, ok = stale.Obj.(ObjectData)
				if !ok {
					logger.Errorf("informer %q reported unknown stale object to be deleted (%T)", resource.Name(), stale.Obj)
					return
				}
			}
			o, err := resource.Wrap(data)
			if err == nil {
				funcs.DeleteFunc(o)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			if funcs.UpdateFunc == nil {
				return
			}
			o, err := resource.Wrap(old.(ObjectData))
			if err == nil {
				n, err := resource.Wrap(new.(ObjectData))
				if err == nil {
					funcs.UpdateFunc(o, n)
				}
			}
		},
	}
}

func convertInfo(resource Interface, funcs *ResourceInfoEventHandlerFuncs) *cache.ResourceEventHandlerFuncs {
	return &cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if funcs.AddFunc == nil {
				return
			}
			o := wrapInfo(resource, obj)
			if o != nil {
				funcs.AddFunc(o)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if funcs.DeleteFunc == nil {
				return
			}
			data, ok := toMinimalObject(resource, obj)
			if !ok {
				stale, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					logger.Errorf("informer %q reported unknown object to be deleted (%T)", resource.Name(), obj)
					return
				}
				if stale.Obj == nil {
					logger.Errorf("informer %q reported no stale object to be deleted", resource.Name())
					return
				}
				data, ok = toMinimalObject(resource, stale.Obj)
				if !ok {
					logger.Errorf("informer %q reported unknown stale object to be deleted (%T)", resource.Name(), stale.Obj)
					return
				}
			}
			o := wrapInfo(resource, data)
			if o != nil {
				funcs.DeleteFunc(o)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			if funcs.UpdateFunc == nil {
				return
			}
			o := wrapInfo(resource, old)
			n := wrapInfo(resource, new)
			if o != nil && n != nil {
				funcs.UpdateFunc(o, n)
			}
		},
	}
}

func toMinimalObject(resource Interface, obj interface{}) (*minimal.MinimalObject, bool) {
	if m, ok := obj.(*minimal.MinimalObject); ok {
		m.SetGroupVersionKind(resource.GroupVersionKind())
		return m, ok
	}
	if meta, ok := obj.(metav1.Object); ok {
		m := minimal.ConvertToMinimalObject("", "", meta)
		m.SetGroupVersionKind(resource.GroupVersionKind())
		return m, true
	}
	return nil, false
}

func wrapInfo(resource Interface, obj interface{}) ObjectInfo {
	m, ok := toMinimalObject(resource, obj)
	if !ok {
		return nil
	}
	return &minimalObjectInfo{
		minimalObject: m,
		cluster:       resource.GetCluster(),
	}
}

type minimalObjectInfo struct {
	minimalObject *minimal.MinimalObject
	cluster       Cluster
}

func (this *minimalObjectInfo) Key() ObjectKey {
	m := this.minimalObject
	return NewKey(m.GroupVersionKind().GroupKind(), m.GetNamespace(), m.GetName())
}

func (this *minimalObjectInfo) Description() string {
	return this.Key().String()
}

func (this *minimalObjectInfo) GetResourceVersion() string {
	return this.minimalObject.ResourceVersion
}

func (this *minimalObjectInfo) GetCluster() Cluster {
	return this.cluster
}
