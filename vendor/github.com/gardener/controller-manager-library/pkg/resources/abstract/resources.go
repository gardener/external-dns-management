/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved.
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

package abstract

import (
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/gardener/controller-manager-library/pkg/resources/errors"

	"github.com/gardener/controller-manager-library/pkg/kutil"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var unstructuredType = reflect.TypeOf(unstructured.Unstructured{})
var unstructuredListType = reflect.TypeOf(unstructured.UnstructuredList{})

type NewResource func(resources Resources, gvk schema.GroupVersionKind, otype reflect.Type, ltype reflect.Type) (Resource, error)

type AbstractResources struct {
	ctx     ResourceContext
	lock    sync.Mutex
	factory Factory

	self Resources

	handlersByObjType          map[reflect.Type]Resource
	handlersByGroupKind        map[schema.GroupKind]Resource
	handlersByGroupVersionKind map[schema.GroupVersionKind]Resource

	unstructuredHandlersByGroupKind        map[schema.GroupKind]Resource
	unstructuredHandlersByGroupVersionKind map[schema.GroupVersionKind]Resource

	newResource NewResource
}

var _ Resources = &AbstractResources{}

func NewAbstractResources(ctx ResourceContext, self Resources, factory Factory) *AbstractResources {
	this := &AbstractResources{
		ctx:                        ctx,
		factory:                    factory,
		self:                       self,
		handlersByObjType:          map[reflect.Type]Resource{},
		handlersByGroupKind:        map[schema.GroupKind]Resource{},
		handlersByGroupVersionKind: map[schema.GroupVersionKind]Resource{},

		unstructuredHandlersByGroupKind:        map[schema.GroupKind]Resource{},
		unstructuredHandlersByGroupVersionKind: map[schema.GroupVersionKind]Resource{},

		newResource: factory.NewResource,
	}
	return this
}

func (this *AbstractResources) Lock() {
	this.lock.Lock()
}

func (this *AbstractResources) Unlock() {
	this.lock.Unlock()
}

func (this *AbstractResources) ResourceContext() ResourceContext {
	return this.ctx
}

func (this *AbstractResources) Scheme() *runtime.Scheme {
	return this.ctx.Scheme()
}

func (this *AbstractResources) Decode(bytes []byte) (ObjectData, error) {
	data, _, err := this.ctx.Decoder().Decode(bytes)
	if err != nil {
		return nil, err
	}
	return data.(ObjectData), nil
}

func (this *AbstractResources) Get(spec interface{}) (Resource, error) {
	switch o := spec.(type) {
	case GroupKindProvider:
		return this.GetByGK(o.GroupKind())
	case runtime.Object:
		return this.GetByExample(o)
	case schema.GroupVersionKind:
		return this.GetByGVK(o)
	case *schema.GroupVersionKind:
		return this.GetByGVK(*o)
	case schema.GroupKind:
		return this.GetByGK(o)
	case *schema.GroupKind:
		return this.GetByGK(*o)

	case ObjectKey:
		return this.GetByGK(o.GroupKind())
	case *ObjectKey:
		return this.GetByGK(o.GroupKind())

	case ClusterObjectKey:
		return this.GetByGK(o.GroupKind())
	case *ClusterObjectKey:
		return this.GetByGK(o.GroupKind())

	default:
		return nil, errors.ErrUnexpectedType.New("object identifier", spec)
	}
}

func (this *AbstractResources) GetByExample(obj runtime.Object) (Resource, error) {
	t := reflect.TypeOf(obj)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	this.Lock()
	defer this.Unlock()
	if handler, ok := this.handlersByObjType[t]; ok {
		return handler, nil
	}

	gvk, err := this.ctx.GetGVK(obj)
	if err != nil {
		return nil, errors.ErrUnknownResource.Wrap(err, "object type", reflect.TypeOf(obj))
	}

	return this._newResource(gvk, t)
}

func (this *AbstractResources) GetByGK(gk schema.GroupKind) (Resource, error) {
	this.Lock()
	defer this.Unlock()

	if handler, ok := this.handlersByGroupKind[gk]; ok {
		return handler, nil
	}

	gvk, err := this.ctx.GetGVKForGK(gk)
	if err != nil {
		return nil, err
	}
	if handler, ok := this.handlersByGroupVersionKind[gvk]; ok {
		this.handlersByGroupKind[gk] = handler
		return handler, nil
	}

	h, err := this.getResource(gvk)
	if err != nil {
		return nil, err
	}
	this.handlersByGroupKind[gk] = h
	this.handlersByGroupVersionKind[gvk] = h
	return h, nil
}

func (this *AbstractResources) GetByGVK(gvk schema.GroupVersionKind) (Resource, error) {
	this.Lock()
	defer this.Unlock()

	if handler, ok := this.handlersByGroupVersionKind[gvk]; ok {
		return handler, nil
	}

	h, err := this.getResource(gvk)
	if err != nil {
		return nil, err
	}
	this.handlersByGroupVersionKind[gvk] = h
	return h, nil
}

func (this *AbstractResources) getResource(gvk schema.GroupVersionKind) (Resource, error) {
	informerType := this.ctx.Scheme().KnownTypes(gvk.GroupVersion())[gvk.Kind]
	if informerType == nil {
		return nil, errors.ErrUnknownResource.New("group version kind", gvk)
	}

	return this._newResource(gvk, informerType)
}

func (this *AbstractResources) GetUnstructured(spec interface{}) (Resource, error) {
	switch o := spec.(type) {
	case GroupVersionKindProvider:
		return this.GetUnstructuredByGVK(o.GroupVersionKind())
	case GroupKindProvider:
		return this.GetUnstructuredByGK(o.GroupKind())
	case schema.GroupVersionKind:
		return this.GetUnstructuredByGVK(o)
	case *schema.GroupVersionKind:
		return this.GetUnstructuredByGVK(*o)
	case schema.GroupKind:
		return this.GetUnstructuredByGK(o)
	case *schema.GroupKind:
		return this.GetUnstructuredByGK(*o)

	case ObjectKey:
		return this.GetUnstructuredByGK(o.GroupKind())
	case *ObjectKey:
		return this.GetUnstructuredByGK(o.GroupKind())

	case ClusterObjectKey:
		return this.GetUnstructuredByGK(o.GroupKind())
	case *ClusterObjectKey:
		return this.GetUnstructuredByGK(o.GroupKind())

	default:
		return nil, errors.ErrUnexpectedType.New("object identifier", spec)
	}
}

func (this *AbstractResources) GetUnstructuredByGK(gk schema.GroupKind) (Resource, error) {
	this.Lock()
	defer this.Unlock()

	if handler, ok := this.unstructuredHandlersByGroupKind[gk]; ok {
		return handler, nil
	}

	gvk, err := this.ctx.GetGVKForGK(gk)
	if err != nil {
		return nil, err
	}

	if handler, ok := this.unstructuredHandlersByGroupVersionKind[gvk]; ok {
		this.unstructuredHandlersByGroupKind[gk] = handler
		return handler, nil
	}

	h, err := this._newResource(gvk, nil)
	if err != nil {
		return nil, err
	}
	this.unstructuredHandlersByGroupKind[gk] = h
	this.unstructuredHandlersByGroupVersionKind[gvk] = h
	return h, nil
}

func (this *AbstractResources) GetUnstructuredByGVK(gvk schema.GroupVersionKind) (Resource, error) {
	this.Lock()
	defer this.Unlock()

	if handler, ok := this.unstructuredHandlersByGroupVersionKind[gvk]; ok {
		return handler, nil
	}

	h, err := this._newResource(gvk, nil)
	if err != nil {
		return nil, err
	}
	this.unstructuredHandlersByGroupVersionKind[gvk] = h
	return h, err
}

func (this *AbstractResources) _newResource(gvk schema.GroupVersionKind, otype reflect.Type) (Resource, error) {

	if otype == nil {
		otype = unstructuredType
	}
	ltype := kutil.DetermineListType(this.ctx.Scheme(), gvk.GroupVersion(), otype)
	if ltype == nil {
		return nil, errors.New(errors.ERR_NO_LIST_TYPE, "cannot determine list type for %s", otype)
	}

	return this.newResource(this.self, gvk, otype, ltype)
}
