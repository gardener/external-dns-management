/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package abstract

import (
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/kutil"
	"github.com/gardener/controller-manager-library/pkg/resources/errors"
)

var unstructuredType = reflect.TypeOf(unstructured.Unstructured{})

type NewResource func(resources Resources, gvk schema.GroupVersionKind, otype reflect.Type, ltype reflect.Type) (Resource, error)

type AbstractResources struct {
	ctx     ResourceContext
	lock    sync.Mutex
	factory Factory

	self Resources

	handlersByObjType map[reflect.Type]Resource

	structuredHandlers   handlersByGxK
	unstructuredHandlers handlersByGxK

	newResource NewResource
}

var _ Resources = &AbstractResources{}

func NewAbstractResources(ctx ResourceContext, self Resources, factory Factory) *AbstractResources {
	this := &AbstractResources{
		ctx:               ctx,
		factory:           factory,
		self:              self,
		handlersByObjType: map[reflect.Type]Resource{},

		newResource: factory.NewResource,
	}
	this.structuredHandlers = newHandlersByGxK(func(gvk schema.GroupVersionKind) (Resource, error) {
		informerType := this.ctx.Scheme().KnownTypes(gvk.GroupVersion())[gvk.Kind]
		if informerType == nil {
			return nil, errors.ErrUnknownResource.New("group version kind", gvk)
		}
		return this._newResource(gvk, informerType)
	})
	this.unstructuredHandlers = newHandlersByGxK(func(gvk schema.GroupVersionKind) (Resource, error) {
		return this._newResource(gvk, nil)
	})
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
	return this.structuredHandlers.getByGK(this.ctx, gk)
}

func (this *AbstractResources) GetByGVK(gvk schema.GroupVersionKind) (Resource, error) {
	return this.structuredHandlers.getByGVK(gvk)
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
	return this.unstructuredHandlers.getByGK(this.ctx, gk)
}

func (this *AbstractResources) GetUnstructuredByGVK(gvk schema.GroupVersionKind) (Resource, error) {
	return this.unstructuredHandlers.getByGVK(gvk)
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

type getResource func(gvk schema.GroupVersionKind) (Resource, error)

type handlersByGxK struct {
	lock                       sync.Mutex
	handlersByGroupKind        map[schema.GroupKind]Resource
	handlersByGroupVersionKind map[schema.GroupVersionKind]Resource
	getResource                getResource
}

func newHandlersByGxK(getResource getResource) handlersByGxK {
	return handlersByGxK{
		handlersByGroupKind:        map[schema.GroupKind]Resource{},
		handlersByGroupVersionKind: map[schema.GroupVersionKind]Resource{},
		getResource:                getResource,
	}
}

func (this *handlersByGxK) getByGK(ctx ResourceContext, gk schema.GroupKind) (Resource, error) {
	this.lock.Lock()
	defer this.lock.Unlock()

	if handler, ok := this.handlersByGroupKind[gk]; ok {
		return handler, nil
	}

	gvk, err := ctx.GetGVKForGK(gk)
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

func (this *handlersByGxK) getByGVK(gvk schema.GroupVersionKind) (Resource, error) {
	this.lock.Lock()
	defer this.lock.Unlock()

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
