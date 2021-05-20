/*
SPDX-FileCopyrightText: The Kubernetes Authors.

SPDX-License-Identifier: Apache-2.0
*/

package resources

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

type GenericInformer interface {
	cache.SharedIndexInformer
	Informer() cache.SharedIndexInformer
	Lister() Lister
}

type genericInformer struct {
	cache.SharedIndexInformer
	resource *Info
}

func (f *genericInformer) Informer() cache.SharedIndexInformer {
	return f.SharedIndexInformer
}

func (f *genericInformer) Lister() Lister {
	return NewLister(f.Informer().GetIndexer(), f.resource)
}

// SharedInformerFactory provides shared informers for resources in all known
// API group versions.
type SharedInformerFactory interface {
	Structured() GenericFilteredInformerFactory
	Unstructured() GenericFilteredInformerFactory
	MinimalObject() GenericFilteredInformerFactory

	InformerForObject(obj runtime.Object) (GenericInformer, error)
	FilteredInformerForObject(obj runtime.Object, namespace string, optionsFunc TweakListOptionsFunc) (GenericInformer, error)

	InformerFor(gvk schema.GroupVersionKind) (GenericInformer, error)
	FilteredInformerFor(gvk schema.GroupVersionKind, namespace string, optionsFunc TweakListOptionsFunc) (GenericInformer, error)

	UnstructuredInformerFor(gvk schema.GroupVersionKind) (GenericInformer, error)
	FilteredUnstructuredInformerFor(gvk schema.GroupVersionKind, namespace string, optionsFunc TweakListOptionsFunc) (GenericInformer, error)

	MinimalObjectInformerFor(gvk schema.GroupVersionKind) (GenericInformer, error)
	FilteredMinimalObjectInformerFor(gvk schema.GroupVersionKind, namespace string, optionsFunc TweakListOptionsFunc) (GenericInformer, error)

	Start(stopCh <-chan struct{})
	WaitForCacheSync(stopCh <-chan struct{})
}

type GenericInformerFactory interface {
	InformerFor(gvk schema.GroupVersionKind) (GenericInformer, error)
	Start(stopCh <-chan struct{})
	WaitForCacheSync(stopCh <-chan struct{})
}

type GenericFilteredInformerFactory interface {
	GenericInformerFactory
	FilteredInformerFor(gvk schema.GroupVersionKind, namespace string, optionsFunc TweakListOptionsFunc) (GenericInformer, error)
	LookupInformerFor(gvk schema.GroupVersionKind, namespace string) (GenericInformer, error)
}

///////////////////////////////////////////////////////////////////////////////
//  informer factory

type sharedInformerFactory struct {
	context       *resourceContext
	structured    *sharedFilteredInformerFactory
	unstructured  *sharedFilteredInformerFactory
	minimalObject *sharedFilteredInformerFactory
}

func newSharedInformerFactory(rctx *resourceContext, defaultResync time.Duration) *sharedInformerFactory {
	return &sharedInformerFactory{
		context:       rctx,
		structured:    newSharedFilteredInformerFactory(rctx, defaultResync, newStructuredListWatchFactory),
		unstructured:  newSharedFilteredInformerFactory(rctx, defaultResync, newUnstructuredListWatchFactory),
		minimalObject: newSharedFilteredInformerFactory(rctx, defaultResync, newMinimalObjectListWatchFactory),
	}
}

func (f *sharedInformerFactory) Structured() GenericFilteredInformerFactory {
	return f.structured
}

func (f *sharedInformerFactory) Unstructured() GenericFilteredInformerFactory {
	return f.unstructured
}

func (f *sharedInformerFactory) MinimalObject() GenericFilteredInformerFactory {
	return f.minimalObject
}

// Start initializes all requested informers.
func (f *sharedInformerFactory) Start(stopCh <-chan struct{}) {
	f.structured.Start(stopCh)
	f.unstructured.Start(stopCh)
	f.minimalObject.Start(stopCh)
}

func (f *sharedInformerFactory) WaitForCacheSync(stopCh <-chan struct{}) {
	f.structured.WaitForCacheSync(stopCh)
	f.unstructured.WaitForCacheSync(stopCh)
	f.minimalObject.WaitForCacheSync(stopCh)
}

func (f *sharedInformerFactory) UnstructuredInformerFor(gvk schema.GroupVersionKind) (GenericInformer, error) {
	return f.FilteredUnstructuredInformerFor(gvk, "", nil)
}

func (f *sharedInformerFactory) FilteredUnstructuredInformerFor(gvk schema.GroupVersionKind, namespace string, optionsFunc TweakListOptionsFunc) (GenericInformer, error) {
	return f.unstructured.informerFor(gvk, namespace, optionsFunc)
}

func (f *sharedInformerFactory) MinimalObjectInformerFor(gvk schema.GroupVersionKind) (GenericInformer, error) {
	return f.FilteredMinimalObjectInformerFor(gvk, "", nil)
}

func (f *sharedInformerFactory) FilteredMinimalObjectInformerFor(gvk schema.GroupVersionKind, namespace string, optionsFunc TweakListOptionsFunc) (GenericInformer, error) {
	return f.minimalObject.informerFor(gvk, namespace, optionsFunc)
}

func (f *sharedInformerFactory) InformerFor(gvk schema.GroupVersionKind) (GenericInformer, error) {
	return f.FilteredInformerFor(gvk, "", nil)
}

func (f *sharedInformerFactory) FilteredInformerFor(gvk schema.GroupVersionKind, namespace string, optionsFunc TweakListOptionsFunc) (GenericInformer, error) {
	return f.structured.informerFor(gvk, namespace, optionsFunc)
}

func (f *sharedInformerFactory) LookupInformerFor(gvk schema.GroupVersionKind, namespace string) (GenericInformer, error) {
	return f.structured.lookupInformerFor(gvk, namespace)
}

func (f *sharedInformerFactory) InformerForObject(obj runtime.Object) (GenericInformer, error) {
	return f.FilteredInformerForObject(obj, "", nil)
}

func (f *sharedInformerFactory) FilteredInformerForObject(obj runtime.Object, namespace string, optionsFunc TweakListOptionsFunc) (GenericInformer, error) {
	informerType := reflect.TypeOf(obj)
	for informerType.Kind() == reflect.Ptr {
		informerType = informerType.Elem()
	}

	gvk, err := f.context.GetGVK(obj)
	if err != nil {
		return nil, err
	}
	return f.FilteredInformerFor(gvk, namespace, optionsFunc)
}

///////////////////////////////////////////////////////////////////////////////
// Shared Filtered Informer Factory

type sharedFilteredInformerFactory struct {
	lock sync.Mutex

	context                    *resourceContext
	defaultResync              time.Duration
	filters                    map[string]*genericInformerFactory
	newListWatchFactoryFactory newListWatchFactoryFactory
}

func newSharedFilteredInformerFactory(rctx *resourceContext, defaultResync time.Duration, ff newListWatchFactoryFactory) *sharedFilteredInformerFactory {
	return &sharedFilteredInformerFactory{
		context:       rctx,
		defaultResync: defaultResync,

		filters:                    make(map[string]*genericInformerFactory),
		newListWatchFactoryFactory: ff,
	}
}

// Start initializes all requested informers.
func (f *sharedFilteredInformerFactory) Start(stopCh <-chan struct{}) {
	for _, i := range f.filters {
		i.Start(stopCh)
	}
}

func (f *sharedFilteredInformerFactory) WaitForCacheSync(stopCh <-chan struct{}) {
	for _, i := range f.filters {
		i.WaitForCacheSync(stopCh)
	}
}

func (f *sharedFilteredInformerFactory) getFactory(namespace string, optionsFunc TweakListOptionsFunc) *genericInformerFactory {
	key := namespace
	if optionsFunc != nil {
		opts := metav1.ListOptions{}
		optionsFunc(&opts)
		key = namespace + opts.String()
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	factory, exists := f.filters[key]
	if !exists {
		factory = newGenericInformerFactory(f.context, f.defaultResync, namespace, optionsFunc)
		f.filters[key] = factory
	}
	return factory
}

func (f *sharedFilteredInformerFactory) queryFactory(namespace string) *genericInformerFactory {
	f.lock.Lock()
	defer f.lock.Unlock()

	factory, _ := f.filters[namespace]
	return factory
}

func (f *sharedFilteredInformerFactory) informerFor(gvk schema.GroupVersionKind, namespace string, optionsFunc TweakListOptionsFunc) (GenericInformer, error) {
	lwFactory, err := f.newListWatchFactoryFactory(f.context, gvk)
	if err != nil {
		return nil, err
	}
	return f.getFactory(namespace, optionsFunc).informerFor(lwFactory)
}

func (f *sharedFilteredInformerFactory) lookupInformerFor(gvk schema.GroupVersionKind, namespace string) (GenericInformer, error) {
	fac := f.queryFactory("")
	if fac != nil {
		i := fac.queryInformerFor(gvk)
		if i != nil {
			return i, nil
		}
	}
	if namespace != "" {
		fac := f.queryFactory(namespace)
		if fac != nil {
			i := fac.queryInformerFor(gvk)
			if i != nil {
				return i, nil
			}
			lwFactory, err := f.newListWatchFactoryFactory(f.context, gvk)
			if err != nil {
				return nil, err
			}
			return fac.informerFor(lwFactory)
		}
	}
	lwFactory, err := f.newListWatchFactoryFactory(f.context, gvk)
	if err != nil {
		return nil, err
	}
	return f.getFactory("", nil).informerFor(lwFactory)
}

func (f *sharedFilteredInformerFactory) InformerFor(gvk schema.GroupVersionKind) (GenericInformer, error) {
	return f.FilteredInformerFor(gvk, "", nil)
}

func (f *sharedFilteredInformerFactory) FilteredInformerFor(gvk schema.GroupVersionKind, namespace string, optionsFunc TweakListOptionsFunc) (GenericInformer, error) {
	return f.informerFor(gvk, namespace, optionsFunc)
}

func (f *sharedFilteredInformerFactory) LookupInformerFor(gvk schema.GroupVersionKind, namespace string) (GenericInformer, error) {
	return f.lookupInformerFor(gvk, namespace)
}

////////////////////////////////////////////////////////////////////////////////
// Watch

type watchWrapper struct {
	ctx        context.Context
	orig       watch.Interface
	origChan   <-chan watch.Event
	resultChan chan watch.Event
}

func NewWatchWrapper(ctx context.Context, orig watch.Interface) watch.Interface {
	logger.Infof("*************** new wrapper ********************")
	w := &watchWrapper{ctx, orig, orig.ResultChan(), make(chan watch.Event)}
	go w.Run()
	return w
}

func (w *watchWrapper) Stop() {
	w.orig.Stop()
}

func (w *watchWrapper) ResultChan() <-chan watch.Event {
	return w.resultChan
}
func (w *watchWrapper) Run() {
loop:
	for {
		select {
		case <-w.ctx.Done():
			break loop
		case e, ok := <-w.origChan:
			if !ok {
				logger.Infof("watch aborted")
				break loop
			} else {
				logger.Infof("WATCH: %#v\n", e)
				w.resultChan <- e
			}
		}
	}
	logger.Infof("stop wrapper ***************")
	close(w.resultChan)
}

var _ watch.Interface = &watchWrapper{}
