/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package annotations

import (
	"fmt"
	"sync"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/abstract"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

const KEY_STATE = "key-watches"

var WatchResourceKind = abstract.NewGroupKind(api.GroupName, api.DNSAnnotationKind)

func GetOrCreateWatches(controller controller.Interface) *State {
	return controller.GetEnvironment().GetOrCreateSharedValue(KEY_STATE, func() interface{} {
		controller.Infof("creating annotation state")
		return NewWatches()
	}).(*State)
}

// Handler is used to notify interesting parties about changes of watch state of an object
type Handler interface {
	ObjectUpdated(key resources.ClusterObjectKey)
}

type Annotations map[string]string

type State struct {
	lock        sync.RWMutex
	annotations map[resources.ClusterObjectKey]*ObjectState
	kinds       map[schema.GroupKind]*ObjectKind
}

type ObjectState struct {
	object      resources.ClusterObjectKey
	controller  string
	annotations map[resources.ClusterObjectKey]*AnnotationState
}

type AnnotationState struct {
	annotation  resources.Object
	annotations Annotations
	time        metav1.Time
}

type ObjectKind struct {
	// map of referenced resources to referencing watch resources
	objects  map[resources.ClusterObjectKey]*ObjectState
	handlers map[Handler]struct{}
}

func (this *ObjectKind) notify(obj resources.ClusterObjectKey) {
	for h := range this.handlers {
		h.ObjectUpdated(obj)
	}
}

func NewWatches() *State {
	return &State{
		annotations: map[resources.ClusterObjectKey]*ObjectState{},
		kinds:       map[schema.GroupKind]*ObjectKind{},
	}
}

func (this *State) RegisterHandler(log logger.LogContext, gk schema.GroupKind, h Handler) {
	this.lock.Lock()
	defer this.lock.Unlock()

	wk := this.assureKind(gk)
	if wk.handlers == nil {
		wk.handlers = map[Handler]struct{}{}
	}
	log.Infof("register annotation handler for %s", gk)
	wk.handlers[h] = struct{}{}
}

func (this *State) UnRegisterHandler(log logger.LogContext, gk schema.GroupKind, h Handler) {
	this.lock.Lock()
	defer this.lock.Unlock()

	wk := this.kinds[gk]
	if wk == nil {
		return
	}
	if wk.handlers == nil {
		return
	}
	log.Infof("remove annotation handler for %s", gk)
	delete(wk.handlers, h)
	this.cleanupKind(gk, wk)
}

func (this *State) Get(gk schema.GroupKind) resources.ClusterObjectKeySet {
	this.lock.RLock()
	defer this.lock.RUnlock()

	set := resources.ClusterObjectKeySet{}
	if kind := this.kinds[gk]; kind != nil {
		for k := range kind.objects {
			set.Add(k)
		}
	}
	return set
}

func (this *State) GetInfoFor(key resources.ClusterObjectKey) Annotations {
	this.lock.RLock()
	defer this.lock.RUnlock()

	if o := this.getStateFor(key); o != nil {
		times := map[string]*metav1.Time{}
		annos := Annotations{}
		for _, w := range o.annotations {
			t := w.time
			for k, v := range w.annotations {
				if times[k] == nil || times[k].Before(&w.time) {
					annos[k] = v
					times[k] = &t
				}
			}
		}
		return annos
	}
	return nil
}

func (this *State) SetActive(key resources.ClusterObjectKey, active bool) error {
	this.lock.RLock()
	defer this.lock.RUnlock()

	if o := this.getStateFor(key); o != nil {
		for _, a := range o.annotations {
			_, err := a.annotation.ModifyStatus(func(data resources.ObjectData) (bool, error) {
				anno := data.(*api.DNSAnnotation)
				if anno.Status.Active != active {
					anno.Status.Active = active
					return true, nil
				}
				return false, nil
			})
			if err != nil {
				logger.Infof("FAILED to update status: %s", err)
				return err
			}
			a.annotation.Data().(*api.DNSAnnotation).Status.Active = active
		}
	}
	return nil
}

func (this *State) getStateFor(key resources.ClusterObjectKey) *ObjectState {
	if kind := this.kinds[key.GroupKind()]; kind != nil {
		return kind.objects[key]
	}
	return nil
}

func (this *State) Add(logger logger.LogContext, annotation resources.Object) error {
	if w, ok := annotation.Data().(*api.DNSAnnotation); ok {
		cluster := annotation.GetCluster().GetId()
		key, err := Ref(cluster, w)
		if err != nil {
			return err
		}
		_, err = annotation.ModifyStatus(func(data resources.ObjectData) (bool, error) {
			msg := ""
			if err != nil {
				msg = fmt.Sprintf("invalid ref: %s", err)
			}
			a := data.(*api.DNSAnnotation)
			if a.Status.Message != msg {
				a.Status.Message = msg
				return true, nil
			}
			return false, nil
		})

		this.lock.Lock()
		defer this.lock.Unlock()

		if old := this.annotations[annotation.ClusterKey()]; old != nil {
			if old.object == key {
				old.annotations[annotation.ClusterKey()].annotation = annotation
				return err
			}
			this.removeAnnotations(logger, annotation.ClusterKey(), old.object)
		}
		if err != nil {
			return err
		}

		this.addAnnotations(logger, annotation.ClusterKey(), key, &AnnotationState{
			annotation,
			w.Spec.Annotations,
			w.CreationTimestamp,
		})
	}
	return nil
}

func (this *State) Remove(logger logger.LogContext, watch resources.ClusterObjectKey) {
	if watch.GroupKind() == WatchResourceKind {
		this.lock.Lock()
		defer this.lock.Unlock()

		if old := this.annotations[watch]; old != nil {
			this.removeAnnotations(logger, watch, old.object)
		}
	}
}

func (this *State) assureKind(gk schema.GroupKind) *ObjectKind {
	wk := this.kinds[gk]

	if wk == nil {
		wk = &ObjectKind{
			objects: map[resources.ClusterObjectKey]*ObjectState{},
		}
		this.kinds[gk] = wk
	}
	return wk
}

func (this *State) cleanupKind(gk schema.GroupKind, wk *ObjectKind) {
	if len(wk.objects) > 0 || len(wk.handlers) > 0 {
		return
	}
	delete(this.kinds, gk)
}

func (this *State) addAnnotations(logger logger.LogContext, watch resources.ClusterObjectKey, obj resources.ClusterObjectKey, info *AnnotationState) {
	wk := this.assureKind(obj.GroupKind())
	o := wk.objects[obj]
	if o == nil {
		logger.Infof("enforce DNS source annotations for %s", obj)
		o = &ObjectState{
			object:      obj,
			controller:  "",
			annotations: map[resources.ClusterObjectKey]*AnnotationState{},
		}
		wk.objects[obj] = o
	} else {
		logger.Infof("enforced DNS source annotations for %s now has %d watch objects", obj, len(o.annotations)+1)
	}
	o.annotations[watch] = info
	this.annotations[watch] = o
	wk.notify(obj)
}

func (this *State) removeAnnotations(logger logger.LogContext, watch resources.ClusterObjectKey, obj resources.ClusterObjectKey) {
	delete(this.annotations, watch)

	wk := this.kinds[obj.GroupKind()]
	if wk == nil {
		return
	}
	o := wk.objects[obj]
	if o == nil {
		return
	}
	delete(o.annotations, watch)
	if len(o.annotations) > 0 {
		logger.Infof("enforced DNS source annotations for %s still has %d watch objects", obj, len(o.annotations))
		return
	}
	logger.Infof("remove annotation enforcement for %s", obj)
	delete(wk.objects, obj)
	this.cleanupKind(obj.GroupKind(), wk)
	wk.notify(obj)
}

func Ref(cluster string, watch *api.DNSAnnotation) (resources.ClusterObjectKey, error) {
	namespace := watch.Namespace
	if watch.Spec.ResourceRef.Namespace != "" {
		namespace = watch.Spec.ResourceRef.Namespace
	}
	gv, err := schema.ParseGroupVersion(watch.Spec.ResourceRef.APIVersion)
	if err != nil {
		return resources.ClusterObjectKey{}, err
	}
	return resources.NewClusterKey(cluster, resources.NewGroupKind(gv.Group, watch.Spec.ResourceRef.Kind), namespace, watch.Spec.ResourceRef.Name), nil
}
