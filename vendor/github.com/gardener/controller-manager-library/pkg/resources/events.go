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
			data, ok := toPartialObjectMetadata(resource, obj)
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
				data, ok = toPartialObjectMetadata(resource, stale.Obj)
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

func toPartialObjectMetadata(resource Interface, obj interface{}) (*metav1.PartialObjectMetadata, bool) {
	if m, ok := obj.(*metav1.PartialObjectMetadata); ok {
		m.SetGroupVersionKind(resource.GroupVersionKind())
		return m, ok
	}
	if meta, ok := obj.(metav1.Object); ok {
		m := minimal.ConvertToPartialObjectMetadata("", "", meta)
		m.SetGroupVersionKind(resource.GroupVersionKind())
		return m, true
	}
	return nil, false
}

func wrapInfo(resource Interface, obj interface{}) ObjectInfo {
	m, ok := toPartialObjectMetadata(resource, obj)
	if !ok {
		return nil
	}
	return &partialObjectMetadataInfo{
		partialObjectMetadata: m,
		cluster:               resource.GetCluster(),
	}
}

type partialObjectMetadataInfo struct {
	partialObjectMetadata *metav1.PartialObjectMetadata
	cluster               Cluster
}

func (this *partialObjectMetadataInfo) Key() ObjectKey {
	m := this.partialObjectMetadata
	return NewKey(m.GroupVersionKind().GroupKind(), m.GetNamespace(), m.GetName())
}

func (this *partialObjectMetadataInfo) Description() string {
	return this.Key().String()
}

func (this *partialObjectMetadataInfo) GetResourceVersion() string {
	return this.partialObjectMetadata.ResourceVersion
}

func (this *partialObjectMetadataInfo) GetCluster() Cluster {
	return this.cluster
}

func WrapPartialMetadataObject(res Interface, info ObjectInfo) Object {
	if m, ok := info.(*partialObjectMetadataInfo); ok {
		if obj, err := res.Wrap(m.partialObjectMetadata); err == nil {
			return obj
		}
	}
	return nil
}
