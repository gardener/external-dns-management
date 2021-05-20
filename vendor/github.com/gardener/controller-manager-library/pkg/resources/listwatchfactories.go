/*
SPDX-FileCopyrightText: The Kubernetes Authors.

SPDX-License-Identifier: Apache-2.0
*/

package resources

import (
	"context"
	"reflect"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/gardener/controller-manager-library/pkg/kutil"
	"github.com/gardener/controller-manager-library/pkg/resources/errors"
	"github.com/gardener/controller-manager-library/pkg/resources/minimal"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

type newListWatchFactoryFactory func(rctx *resourceContext, gvk schema.GroupVersionKind) (listWatchFactory, error)

// listWatchFactory knows how to create a ListWatch
type listWatchFactory interface {
	CreateListWatch(ctx context.Context, namespace string, optionsFunc TweakListOptionsFunc) (*cache.ListWatch, error)
	GroupVersionKind() schema.GroupVersionKind
	Info() *Info
	Resync() time.Duration
	ExampleObject() runtime.Object
	ElemType() reflect.Type
	ListType() reflect.Type
}

///////////////////////////////////////////////////////////////////////////////
// listWatchFactoryBase

func newListWatchFactoryBase(rctx *resourceContext, gvk schema.GroupVersionKind, elemType reflect.Type) (*listWatchFactoryBase, error) {
	info, err := rctx.Get(gvk)
	if err != nil {
		return nil, err
	}
	listType := kutil.DetermineListType(rctx.Scheme(), gvk.GroupVersion(), elemType)
	if listType == nil {
		return nil, errors.New(errors.ERR_NO_LIST_TYPE, "cannot determine list type for %s", elemType)
	}
	return &listWatchFactoryBase{
		rctx:     rctx,
		info:     info,
		gvk:      gvk,
		elemType: elemType,
		listType: listType,
	}, nil
}

type listWatchFactoryBase struct {
	rctx     *resourceContext
	info     *Info
	gvk      schema.GroupVersionKind
	elemType reflect.Type
	listType reflect.Type
}

func (f *listWatchFactoryBase) GroupVersionKind() schema.GroupVersionKind {
	return f.gvk
}
func (f *listWatchFactoryBase) Info() *Info {
	return f.info
}
func (f *listWatchFactoryBase) Resync() time.Duration {
	return f.rctx.defaultResync
}
func (f *listWatchFactoryBase) ExampleObject() runtime.Object {
	return reflect.New(f.elemType).Interface().(runtime.Object)
}
func (f *listWatchFactoryBase) ElemType() reflect.Type {
	return f.elemType
}
func (f *listWatchFactoryBase) ListType() reflect.Type {
	return f.listType
}

func (f *listWatchFactoryBase) structuredListFunc(ctx context.Context, namespace string, optionsFunc TweakListOptionsFunc, client rest.Interface) cache.ListFunc {
	res := f.Info()
	parametercodec := f.rctx.Clients.parametercodec
	listType := f.ListType()

	return func(options metav1.ListOptions) (runtime.Object, error) {
		if optionsFunc != nil {
			optionsFunc(&options)
		}

		result := reflect.New(listType).Interface().(runtime.Object)
		r := client.Get().
			Resource(res.Name()).
			VersionedParams(&options, parametercodec)
		if res.Namespaced() {
			r = r.Namespace(namespace)
		}

		return result, r.Do(ctx).Into(result)
	}
}

func (f *listWatchFactoryBase) structuredWatchFunc(ctx context.Context, namespace string, optionsFunc TweakListOptionsFunc, client rest.Interface) cache.WatchFunc {
	res := f.Info()
	parametercodec := f.rctx.Clients.parametercodec

	return func(options metav1.ListOptions) (watch.Interface, error) {
		options.Watch = true
		options.AllowWatchBookmarks = true
		if optionsFunc != nil {
			optionsFunc(&options)
		}
		r := client.Get().
			Resource(res.Name()).
			VersionedParams(&options, parametercodec)
		if res.Namespaced() {
			r = r.Namespace(namespace)
		}

		return r.Watch(ctx)
	}
}

func (f *listWatchFactoryBase) dynamicListFunc(ctx context.Context, namespace string, optionsFunc TweakListOptionsFunc, client dynamic.Interface) cache.ListFunc {
	res := f.Info()
	return func(options metav1.ListOptions) (runtime.Object, error) {
		if optionsFunc != nil {
			optionsFunc(&options)
		}
		if res.Namespaced() && namespace != "" {
			return client.Resource(res.GroupVersionResource()).Namespace(namespace).List(ctx, options)
		} else {
			return client.Resource(res.GroupVersionResource()).List(ctx, options)
		}
	}
}

func (f *listWatchFactoryBase) dynamicWatchFunc(ctx context.Context, namespace string, optionsFunc TweakListOptionsFunc, client dynamic.Interface) cache.WatchFunc {
	res := f.Info()
	return func(options metav1.ListOptions) (watch.Interface, error) {
		options.Watch = true
		options.AllowWatchBookmarks = true
		if optionsFunc != nil {
			optionsFunc(&options)
		}
		if res.Namespaced() && namespace != "" {
			return client.Resource(res.GroupVersionResource()).Namespace(namespace).Watch(ctx, options)
		} else {
			return client.Resource(res.GroupVersionResource()).Watch(ctx, options)
		}
	}
}

///////////////////////////////////////////////////////////////////////////////
// structuredListWatchFactory

func newStructuredListWatchFactory(rctx *resourceContext, gvk schema.GroupVersionKind) (listWatchFactory, error) {
	elemType := rctx.KnownTypes(gvk.GroupVersion())[gvk.Kind]
	if elemType == nil {
		return nil, errors.ErrUnknownResource.New("group version kind", gvk)
	}
	base, err := newListWatchFactoryBase(rctx, gvk, elemType)
	if err != nil {
		return nil, err
	}
	return &structuredListWatchFactory{*base}, nil
}

type structuredListWatchFactory struct {
	listWatchFactoryBase
}

func (f *structuredListWatchFactory) CreateListWatch(ctx context.Context, namespace string, optionsFunc TweakListOptionsFunc) (*cache.ListWatch, error) {
	res := f.Info()
	client, err := f.rctx.GetClient(res.GroupVersion())
	if err != nil {
		return nil, err
	}
	return &cache.ListWatch{
		ListFunc:  f.structuredListFunc(ctx, namespace, optionsFunc, client),
		WatchFunc: f.structuredWatchFunc(ctx, namespace, optionsFunc, client),
	}, nil
}

///////////////////////////////////////////////////////////////////////////////
// unstructuredListWatchFactory

func newUnstructuredListWatchFactory(rctx *resourceContext, gvk schema.GroupVersionKind) (listWatchFactory, error) {
	unstructuredType := reflect.TypeOf(unstructured.Unstructured{})
	base, err := newListWatchFactoryBase(rctx, gvk, unstructuredType)
	if err != nil {
		return nil, err
	}
	return &unstructuredListWatchFactory{*base}, nil
}

type unstructuredListWatchFactory struct {
	listWatchFactoryBase
}

func (f *unstructuredListWatchFactory) CreateListWatch(ctx context.Context, namespace string, optionsFunc TweakListOptionsFunc) (*cache.ListWatch, error) {
	cfg := f.rctx.config
	client, err := dynamic.NewForConfig(&cfg)
	if err != nil {
		return nil, err
	}

	return &cache.ListWatch{
		ListFunc:  f.dynamicListFunc(ctx, namespace, optionsFunc, client),
		WatchFunc: f.dynamicWatchFunc(ctx, namespace, optionsFunc, client),
	}, nil
}

///////////////////////////////////////////////////////////////////////////////
// newMinimalObjectListWatchFactory

func newMinimalObjectListWatchFactory(rctx *resourceContext, gvk schema.GroupVersionKind) (listWatchFactory, error) {
	elemType := reflect.TypeOf(minimal.MinimalObject{})
	base, err := newListWatchFactoryBase(rctx, gvk, elemType)
	if err != nil {
		return nil, err
	}
	return &minimalObjectListWatchFactory{*base}, nil
}

type minimalObjectListWatchFactory struct {
	listWatchFactoryBase
}

func (f *minimalObjectListWatchFactory) CreateListWatch(ctx context.Context, namespace string, optionsFunc TweakListOptionsFunc) (*cache.ListWatch, error) {
	res := f.Info()
	client, err := f.rctx.GetClient(res.GroupVersion())
	if err != nil {
		return nil, err
	}
	cfg := f.rctx.config
	dynamicClient, err := dynamic.NewForConfig(&cfg)
	if err != nil {
		return nil, err
	}

	innerWatchFunc := f.dynamicWatchFunc(ctx, namespace, optionsFunc, dynamicClient)

	return &cache.ListWatch{
		ListFunc: f.structuredListFunc(ctx, namespace, optionsFunc, client),
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			w, err := innerWatchFunc(options)
			if err != nil {
				return nil, err
			}
			return minimal.MinimalWatchFilter(w), nil
		},
	}, nil
}
