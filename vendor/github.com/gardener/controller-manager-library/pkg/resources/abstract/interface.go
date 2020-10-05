/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package abstract

import (
	"context"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ClusterGroupKind struct {
	Cluster string
	schema.GroupKind
}

func NewClusterGroupKind(cluster string, gk schema.GroupKind) ClusterGroupKind {
	return ClusterGroupKind{cluster, gk}
}

func (cgk ClusterGroupKind) Empty() bool {
	return len(cgk.Cluster) == 0 && cgk.GroupKind.Empty()
}

func (cgk ClusterGroupKind) String() string {
	return cgk.Cluster + "/" + cgk.GroupKind.String()
}

type GroupKindProvider interface {
	GroupKind() schema.GroupKind
}

type GroupVersionKindProvider interface {
	GroupVersionKind() schema.GroupVersionKind
}

// objectKey is just used to allow a method ObjectKey for ClusterObjectKey
type objectKey struct {
	ObjectKey
}

type ClusterObjectKey struct {
	cluster string
	objectKey
}

// ObjectKey used for worker queues.
type ObjectKey struct {
	groupKind schema.GroupKind
	name      ObjectName
}

type ResourceContext interface {
	context.Context

	Scheme() *runtime.Scheme
	Decoder() *Decoder

	ObjectKinds(obj runtime.Object) ([]schema.GroupVersionKind, bool, error)
	KnownTypes(gv schema.GroupVersion) map[string]reflect.Type

	GetGroups() []schema.GroupVersion
	GetGVK(obj runtime.Object) (schema.GroupVersionKind, error)
	GetGVKForGK(gk schema.GroupKind) (schema.GroupVersionKind, error)
}

type Resources interface {
	Scheme() *runtime.Scheme
}

type Resource interface {
	GroupKindProvider
	GroupVersionKind() schema.GroupVersionKind

	ObjectType() reflect.Type
	ListType() reflect.Type
}

func Everything() labels.Selector {
	return labels.Everything()
}

type Object interface {
	metav1.Object
	GroupKindProvider
	// runtime.ObjectData

	GroupVersionKind() schema.GroupVersionKind
	ObjectName() ObjectName
	Data() ObjectData
	StatusField() interface{}
	Key() ObjectKey

	IsA(spec interface{}) bool

	Description() string
	HasFinalizer(key string) bool
	SetFinalizer(key string) error
	RemoveFinalizer(key string) error

	GetLabel(name string) string
	GetAnnotation(name string) string

	IsDeleting() bool

	GetOwnerReference() *metav1.OwnerReference
}

type ObjectMatcher func(Object) bool

type ObjectNameProvider interface {
	Namespace() string
	Name() string
}

type ObjectName interface {
	ObjectNameProvider

	ForGroupKind(gk schema.GroupKind) ObjectKey
	String() string
}

type ObjectDataName interface {
	GetName() string
	GetNamespace() string
}

type ObjectData interface {
	metav1.Object
	runtime.Object
}
