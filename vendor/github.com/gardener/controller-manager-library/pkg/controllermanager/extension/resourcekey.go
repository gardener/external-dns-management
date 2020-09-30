/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package extension

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources"
)

// ResourceKey implementations are used as key and MUST therefore be value types
type ResourceKey interface {
	GroupKind() schema.GroupKind
	String() string
}

type resourceKey struct {
	key schema.GroupKind
}

func NewResourceKey(group, kind string) ResourceKey {
	if group == "core" {
		group = corev1.GroupName
	}
	return resourceKey{schema.GroupKind{Group: group, Kind: kind}}
}
func (k resourceKey) GroupKind() schema.GroupKind {
	return k.key
}
func (k resourceKey) String() string {
	if k.key.Group == corev1.GroupName {
		return fmt.Sprintf("%s/%s", "core", k.key.Kind)

	}
	return fmt.Sprintf("%s/%s", k.key.Group, k.key.Kind)
}

func GetResourceKey(objspec interface{}) ResourceKey {
	switch s := objspec.(type) {
	case resources.Object:
		return NewResourceKey(s.GroupKind().Group, s.GroupKind().Kind)
	case resources.ObjectKey:
		return NewResourceKey(s.Group(), s.Kind())
	case schema.GroupKind:
		return NewResourceKey(s.Group, s.Kind)
	case schema.GroupVersionKind:
		return NewResourceKey(s.Group, s.Kind)
	default:
		panic(fmt.Errorf("invalid object spec %T for resource key", objspec))
	}
}

////////////////////////////////////////////////////////////////////////////////

// ClusterResourceKey implementations are used as key and MUST therefore be value types
type ClusterResourceKey interface {
	GroupKind() schema.GroupKind
	ClusterId() string
	String() string
}

type clusterResourceKey struct {
	resourceKey
	clusterid string
}

func NewClusterResourceKey(clusterid, group, kind string) ClusterResourceKey {
	if group == "core" {
		group = corev1.GroupName
	}
	return clusterResourceKey{resourceKey: resourceKey{schema.GroupKind{Group: group, Kind: kind}}, clusterid: clusterid}
}

func (k clusterResourceKey) ClusterId() string {
	return k.clusterid
}
func (k clusterResourceKey) String() string {
	return fmt.Sprintf("%s/%s", k.clusterid, k.resourceKey.String())
}

func GetClusterResourceKey(objspec interface{}) ResourceKey {
	switch s := objspec.(type) {
	case resources.Object:
		return NewClusterResourceKey(s.GetCluster().GetId(), s.GroupKind().Group, s.GroupKind().Kind)
	case resources.ClusterObjectKey:
		return NewClusterResourceKey(s.Cluster(), s.Group(), s.Kind())
	default:
		panic(fmt.Errorf("invalid object spec %T for cluster resource key", objspec))
	}
}
