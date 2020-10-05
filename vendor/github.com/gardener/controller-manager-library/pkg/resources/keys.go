/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources/abstract"
)

////////////////////////////////////////////////////////////////////////////////
// ClusterObjectKey
////////////////////////////////////////////////////////////////////////////////

func EqualsClusterObjectKey(a, b ClusterObjectKey) bool {
	return abstract.EqualsClusterObjectKey(a, b)
}

func NewClusterKeyForObject(cluster string, key ObjectKey) ClusterObjectKey {
	return abstract.NewClusterKeyForObject(cluster, key)
}

func NewClusterKey(cluster string, groupKind schema.GroupKind, namespace, name string) ClusterObjectKey {
	return abstract.NewClusterKey(cluster, groupKind, namespace, name)
}

func ParseClusterObjectKey(clusterid string, key string) (ClusterObjectKey, error) {
	return abstract.ParseClusterObjectKey(clusterid, key)
}

////////////////////////////////////////////////////////////////////////////////
// Cluster Object Key Set
////////////////////////////////////////////////////////////////////////////////

type ClusterObjectKeys = abstract.ClusterObjectKeys
type ClusterObjectKeySet = abstract.ClusterObjectKeySet

func NewClusterObjectKeySet(a ...ClusterObjectKey) ClusterObjectKeySet {
	return abstract.NewClusterObjectKeySet(a...)
}

func NewClusterObjectKeySetByArray(a []ClusterObjectKey) ClusterObjectKeySet {
	return abstract.NewClusterObjectKeySetByArray(a)
}

func NewClusterObjectKeSetBySets(sets ...ClusterObjectKeySet) ClusterObjectKeySet {
	return abstract.NewClusterObjectKeSetBySets(sets...)
}

////////////////////////////////////////////////////////////////////////////////
// Object Key
////////////////////////////////////////////////////////////////////////////////

func EqualsObjectKey(a, b ObjectKey) bool {
	return abstract.EqualsObjectKey(a, b)
}

func NewKey(groupKind schema.GroupKind, namespace, name string) ObjectKey {
	return abstract.NewKey(groupKind, namespace, name)
}

func NewKeyForData(data ObjectData) ObjectKey {
	return abstract.NewKeyForData(data)
}

////////////////////////////////////////////////////////////////////////////////
// Group Kind
////////////////////////////////////////////////////////////////////////////////

func NewGroupKind(group, kind string) schema.GroupKind {
	return abstract.NewGroupKind(group, kind)
}

////////////////////////////////////////////////////////////////////////////////
// Group Kind Set
////////////////////////////////////////////////////////////////////////////////

type GroupKindSet = abstract.GroupKindSet

func NewGroupKindSet(a ...schema.GroupKind) GroupKindSet {
	return abstract.NewGroupKindSet(a...)
}

func NewGroupKindSetByArray(a []schema.GroupKind) GroupKindSet {
	return abstract.NewGroupKindSetByArray(a)
}

func NewGroupKindSetBySets(sets ...GroupKindSet) GroupKindSet {
	return abstract.NewGroupKindSetBySets(sets...)
}

////////////////////////////////////////////////////////////////////////////////
// Cluster Group Kind Set
////////////////////////////////////////////////////////////////////////////////

type ClusterGroupKindSet = abstract.ClusterGroupKindSet

func NewClusterGroupKindSet(a ...ClusterGroupKind) ClusterGroupKindSet {
	return abstract.NewClusterGroupKindSet(a...)
}

func NewClusterGroupKindSetByArray(a []ClusterGroupKind) ClusterGroupKindSet {
	return abstract.NewClusterGroupKindSetByArray(a)
}

func NewClusterGroupKindSetBySets(sets ...ClusterGroupKindSet) ClusterGroupKindSet {
	return abstract.NewClusterGroupKindSetBySets(sets...)
}

////////////////////////////////////////////////////////////////////////////////
// Object Name
////////////////////////////////////////////////////////////////////////////////

func EqualsObjectName(a, b ObjectName) bool {
	return abstract.EqualsObjectName(a, b)
}

func NewObjectNameFor(p ObjectNameProvider) abstract.GenericObjectName {
	return abstract.NewObjectNameFor(p)
}

func NewObjectNameForData(data ObjectData) ObjectName {
	return abstract.NewObjectNameForData(data)
}

func NewObjectName(names ...string) abstract.GenericObjectName {
	return abstract.NewObjectName(names...)
}

func ParseObjectName(name string) (abstract.GenericObjectName, error) {
	return abstract.ParseObjectName(name)
}

////////////////////////////////////////////////////////////////////////////////
// Object Name Set
////////////////////////////////////////////////////////////////////////////////

type ObjectNameSet = abstract.ObjectNameSet

func NewObjectNameSet(a ...ObjectName) ObjectNameSet {
	return abstract.NewObjectNameSet(a...)
}

func NewObjectNameSetByArray(a []ObjectName) ObjectNameSet {
	return abstract.NewObjectNameSetByArray(a)
}

func NewObjectNameSetBySets(sets ...ObjectNameSet) ObjectNameSet {
	return abstract.NewObjectNameSetBySets(sets...)
}
