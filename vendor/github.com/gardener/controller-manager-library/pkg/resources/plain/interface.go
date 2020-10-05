/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package plain

import (
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/abstract"
)

type GroupKindProvider = abstract.GroupKindProvider
type ClusterObjectKey = abstract.ClusterObjectKey
type ObjectKey = abstract.ObjectKey
type ObjectMatcher = abstract.ObjectMatcher
type ObjectNameProvider = abstract.ObjectNameProvider
type ObjectName = abstract.ObjectName
type ObjectDataName = abstract.ObjectDataName
type GenericObjectName = abstract.GenericObjectName
type ObjectData = abstract.ObjectData
type Decoder = abstract.Decoder

type ResourcesSource interface {
	Resources() Resources
}

type Object interface {
	abstract.Object
	// runtime.ObjectData
	ResourcesSource

	DeepCopy() Object
	GetResource() Interface

	ForCluster(cluster resources.Cluster) (resources.Object, error)

	CreateIn(cluster resources.Cluster) error
	CreateOrUpdateIn(cluster resources.Cluster) error
	UpdateIn(cluster resources.Cluster) error
	ModifiyIn(cluster resources.Cluster, modifier resources.Modifier) (bool, error)
	DeleteIn(cluster resources.Cluster) error
	SetFinalizerIn(cluster resources.Cluster, key string) error
}

type Interface interface {
	GroupKindProvider
	ResourcesSource

	GroupVersionKind() schema.GroupVersionKind

	Wrap(ObjectData) (Object, error)
	New(ObjectName) Object
	IsUnstructured() bool

	ObjectType() reflect.Type
	ListType() reflect.Type
}

type Resources interface {
	ResourcesSource

	ResourceContext() ResourceContext

	Get(spec interface{}) (Interface, error)
	GetByExample(obj runtime.Object) (Interface, error)
	GetByGK(gk schema.GroupKind) (Interface, error)
	GetByGVK(gvk schema.GroupVersionKind) (Interface, error)

	GetUnstructured(spec interface{}) (Interface, error)
	GetUnstructuredByGK(gk schema.GroupKind) (Interface, error)
	GetUnstructuredByGVK(gvk schema.GroupVersionKind) (Interface, error)

	Wrap(obj ObjectData) (Object, error)
	Decode([]byte) (Object, error)
}
