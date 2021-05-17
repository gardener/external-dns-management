/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"

	"k8s.io/apimachinery/pkg/runtime/schema"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type internalInformerFactory interface {
	informerFor(lwFactory listWatchFactory) (GenericInformer, error)

	Start(stopCh <-chan struct{})
	WaitForCacheSync(stopCh <-chan struct{})
}

///////////////////////////////////////////////////////////////////////////////
// Generic Informer Factory

type genericInformerFactory struct {
	lock sync.Mutex

	context     *resourceContext
	optionsFunc TweakListOptionsFunc
	namespace   string

	defaultResync time.Duration
	informers     map[schema.GroupVersionKind]GenericInformer
	// startedInformers is used for tracking which informers have been started.
	// This allows Start() to be called multiple times safely.
	startedInformers map[schema.GroupVersionKind]bool
}

var _ internalInformerFactory = &genericInformerFactory{}

func newGenericInformerFactory(rctx *resourceContext, defaultResync time.Duration, namespace string, optionsFunc TweakListOptionsFunc) *genericInformerFactory {
	return &genericInformerFactory{
		context:       rctx,
		defaultResync: defaultResync,
		optionsFunc:   optionsFunc,
		namespace:     namespace,

		informers:        make(map[schema.GroupVersionKind]GenericInformer),
		startedInformers: make(map[schema.GroupVersionKind]bool),
	}
}

// Start initializes all requested informers.
func (f *genericInformerFactory) Start(stopCh <-chan struct{}) {
	f.lock.Lock()
	defer f.lock.Unlock()

	for informerType, informer := range f.informers {
		if !f.startedInformers[informerType] {
			go informer.Run(stopCh)
			f.startedInformers[informerType] = true
		}
	}
}

// WaitForCacheSync waits for all started informers' cache were synced.
func (f *genericInformerFactory) WaitForCacheSync(stopCh <-chan struct{}) {
	informers := func() map[schema.GroupVersionKind]cache.SharedIndexInformer {
		f.lock.Lock()
		defer f.lock.Unlock()

		informers := map[schema.GroupVersionKind]cache.SharedIndexInformer{}
		for informerType, informer := range f.informers {
			if f.startedInformers[informerType] {
				informers[informerType] = informer
			}
		}
		return informers
	}()

	for _, informer := range informers {
		cache.WaitForCacheSync(stopCh, informer.HasSynced)
	}
}

func (f *genericInformerFactory) informerFor(lwFactory listWatchFactory) (GenericInformer, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	informer, exists := f.informers[lwFactory.GroupVersionKind()]
	if exists {
		return informer, nil
	}

	var err error
	informer, err = f.newInformer(lwFactory)
	if err != nil {
		return nil, err
	}
	f.informers[lwFactory.GroupVersionKind()] = informer

	return informer, nil
}

func (f *genericInformerFactory) queryInformerFor(gvk schema.GroupVersionKind) GenericInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informer, exists := f.informers[gvk]
	if exists {
		return informer
	}
	return nil
}

func (f *genericInformerFactory) getClient(gv schema.GroupVersion) (restclient.Interface, error) {
	return f.context.GetClient(gv)
}

func (f *genericInformerFactory) newInformer(lw listWatchFactory) (GenericInformer, error) {
	logger.Infof("new generic informer for %s (%s) %s (%d seconds)", lw.ElemType(), lw.GroupVersionKind(), lw.ListType(), lw.Resync()/time.Second)

	listWatch, err := lw.CreateListWatch(context.Background(), f.namespace, f.optionsFunc)
	if err != nil {
		return nil, err
	}
	informer := cache.NewSharedIndexInformer(listWatch, lw.ExampleObject(), resyncPeriod(lw.Resync())(),
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	return &genericInformer{informer, lw.Info()}, nil
}

// resyncPeriod returns a function which generates a duration each time it is
// invoked; this is so that multiple controllers don't get into lock-step and all
// hammer the apiserver with list requests simultaneously.
func resyncPeriod(resync time.Duration) func() time.Duration {
	return func() time.Duration {
		// the factor will fall into [0.9, 1.1)
		factor := rand.Float64()/5.0 + 0.9
		return time.Duration(float64(resync.Nanoseconds()) * factor)
	}
}
