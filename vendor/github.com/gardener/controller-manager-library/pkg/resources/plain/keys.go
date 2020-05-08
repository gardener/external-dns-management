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

package plain

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources/abstract"
)

func EqualsObjectName(a, b ObjectName) bool {
	return abstract.EqualsObjectName(a, b)
}

func NewKey(groupKind schema.GroupKind, namespace, name string) ObjectKey {
	return abstract.NewKey(groupKind, namespace, name)
}

func NewGroupKind(group, kind string) schema.GroupKind {
	return abstract.NewGroupKind(group, kind)
}

////////////////////////////////////////////////////////////////////////////////
// ClusterObjectKey
////////////////////////////////////////////////////////////////////////////////

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
// Object Name
////////////////////////////////////////////////////////////////////////////////

func NewObjectNameFor(p ObjectNameProvider) abstract.GenericObjectName {
	return abstract.NewObjectNameFor(p)
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
