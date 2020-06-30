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

package resources

import (
	"github.com/Masterminds/semver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"github.com/gardener/controller-manager-library/pkg/resources/abstract"
)

type KeyFilter func(key ClusterObjectKey) bool
type ObjectFilter func(obj Object) bool
type GroupKindProvider = abstract.GroupKindProvider
type ClusterGroupKind = abstract.ClusterGroupKind
type ClusterObjectKey = abstract.ClusterObjectKey
type ObjectKey = abstract.ObjectKey
type ObjectMatcher func(Object) bool
type ObjectNameProvider = abstract.ObjectNameProvider
type ObjectName = abstract.ObjectName
type ObjectDataName = abstract.ObjectDataName
type GenericObjectName = abstract.GenericObjectName
type ObjectData = abstract.ObjectData

// TweakListOptionsFunc defines the signature of a helper function
// that wants to provide more listing options to API
type TweakListOptionsFunc func(*metav1.ListOptions)

type ResourcesSource interface {
	Resources() Resources
}

type ClusterSource interface {
	GetCluster() Cluster
}

type Cluster interface {
	ResourcesSource
	ClusterSource
	GetServerVersion() *semver.Version

	GetName() string
	GetId() string
	Config() restclient.Config

	GetAttr(key interface{}) interface{}
	SetAttr(key, value interface{})
}

/////////////////////////////////////////////////////////////////////////////////

type EventRecorder interface {
	Event(eventtype, reason, message string)

	// Eventf is just like Event, but with Sprintf for the message field.
	Eventf(eventtype, reason, messageFmt string, args ...interface{})

	// PastEventf is just like Eventf, but with an option to specify the event's 'timestamp' field.
	PastEventf(timestamp metav1.Time, eventtype, reason, messageFmt string, args ...interface{})

	// AnnotatedEventf is just like eventf, but with annotations attached
	AnnotatedEventf(annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{})
}

type ResourceEventHandlerFuncs struct {
	AddFunc    func(obj Object)
	UpdateFunc func(oldObj, newObj Object)
	DeleteFunc func(obj Object)
}

type Modifier func(ObjectData) (bool, error)

type Object interface {
	abstract.Object
	// runtime.ObjectData
	EventRecorder
	ResourcesSource
	ClusterSource

	DeepCopy() Object
	ClusterKey() ClusterObjectKey
	IsCoLocatedTo(o Object) bool

	GetResource() Interface

	Create() error
	CreateOrUpdate() error
	Delete() error
	Update() error
	UpdateStatus() error
	Modify(modifier Modifier) (bool, error)
	ModifyStatus(modifier Modifier) (bool, error)
	CreateOrModify(modifier Modifier) (bool, error)
	UpdateFromCache() error

	GetOwners(kinds ...schema.GroupKind) ClusterObjectKeySet
	AddOwner(Object) bool
	RemoveOwner(Object) bool
}

type Interface interface {
	abstract.Resource
	ClusterSource
	ResourcesSource

	Name() string
	Namespaced() bool
	Info() *Info
	ResourceContext() ResourceContext
	AddSelectedEventHandler(eventHandlers ResourceEventHandlerFuncs, namespace string, optionsFunc TweakListOptionsFunc) error
	AddEventHandler(eventHandlers ResourceEventHandlerFuncs) error
	AddRawEventHandler(handlers cache.ResourceEventHandlerFuncs) error

	Wrap(ObjectData) (Object, error)
	New(ObjectName) Object

	GetInto(ObjectName, ObjectData) (Object, error)

	GetCached(interface{}) (Object, error)
	// GET_ deprecrated: use Get
	Get_(obj interface{}) (Object, error)
	Get(obj interface{}) (Object, error)
	ListCached(selector labels.Selector) ([]Object, error)
	List(opts metav1.ListOptions) (ret []Object, err error)
	Create(ObjectData) (Object, error)
	CreateOrUpdate(obj ObjectData) (Object, error)
	Update(ObjectData) (Object, error)
	Modify(obj ObjectData, modifier Modifier) (ObjectData, bool, error)
	ModifyByName(obj ObjectDataName, modifier Modifier) (Object, bool, error)
	CreateOrModifyByName(obj ObjectDataName, modifier Modifier) (Object, bool, error)
	ModifyStatus(obj ObjectData, modifier Modifier) (ObjectData, bool, error)
	ModifyStatusByName(obj ObjectDataName, modifier Modifier) (Object, bool, error)
	Delete(ObjectData) error
	DeleteByName(ObjectDataName) error

	NormalEventf(name ObjectDataName, reason, msgfmt string, args ...interface{})
	WarningEventf(name ObjectDataName, reason, msgfmt string, args ...interface{})

	Namespace(name string) Namespaced

	IsUnstructured() bool
}

type Namespaced interface {
	ListCached(selector labels.Selector) ([]Object, error)
	List(opts metav1.ListOptions) (ret []Object, err error)
	GetCached(name string) (Object, error)
	Get(name string) (Object, error)
}

type Resources interface {
	abstract.Resources
	ResourcesSource
	record.EventRecorder

	Get(interface{}) (Interface, error)
	GetByExample(obj runtime.Object) (Interface, error)
	GetByGK(gk schema.GroupKind) (Interface, error)
	GetByGVK(gvk schema.GroupVersionKind) (Interface, error)

	GetUnstructured(spec interface{}) (Interface, error)
	GetUnstructuredByGK(gk schema.GroupKind) (Interface, error)
	GetUnstructuredByGVK(gvk schema.GroupVersionKind) (Interface, error)

	Wrap(obj ObjectData) (Object, error)
	Decode(bytes []byte) (Object, error)

	GetObjectInto(ObjectName, ObjectData) (Object, error)

	GetObject(spec interface{}) (Object, error)
	GetCachedObject(spec interface{}) (Object, error)

	CreateObject(ObjectData) (Object, error)
	CreateOrUpdateObject(obj ObjectData) (Object, error)

	UpdateObject(obj ObjectData) (Object, error)
	ModifyObject(obj ObjectData, modifier func(data ObjectData) (bool, error)) (ObjectData, bool, error)
	ModifyObjectStatus(obj ObjectData, modifier func(data ObjectData) (bool, error)) (ObjectData, bool, error)
	DeleteObject(obj ObjectData) error
}
