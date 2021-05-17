/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 *
 */

package filter

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources"
)

type KeyFilter = resources.KeyFilter
type ClusterObjectKey = resources.ClusterObjectKey

/////////////////////////////////////////////////////////////////////////////////

func All(key ClusterObjectKey) bool {
	return true
}

func None(key ClusterObjectKey) bool {
	return false
}

func GroupKindFilter(gk ...schema.GroupKind) KeyFilter {
	return GroupKindFilterBySet(resources.NewGroupKindSet(gk...))
}

func GroupKindFilterBySet(gks resources.GroupKindSet) KeyFilter {
	return func(key ClusterObjectKey) bool {
		return gks.Contains(key.GroupKind())
	}
}

func ClusterGroupKindFilter(cgk ...resources.ClusterGroupKind) KeyFilter {
	return ClusterGroupKindFilterBySet(resources.NewClusterGroupKindSet(cgk...))
}

func ClusterGroupKindFilterBySet(gks resources.ClusterGroupKindSet) KeyFilter {
	return func(key ClusterObjectKey) bool {
		return gks.Contains(key.ClusterGroupKind())
	}
}

func Or(filters ...KeyFilter) KeyFilter {
	return func(key ClusterObjectKey) bool {
		for _, f := range filters {
			if f(key) {
				return true
			}
		}
		return false
	}
}

func And(filters ...KeyFilter) KeyFilter {
	return func(key ClusterObjectKey) bool {
		if len(filters) == 0 {
			return false
		}
		for _, f := range filters {
			if !f(key) {
				return false
			}
		}
		return true
	}
}

func Not(filter KeyFilter) KeyFilter {
	return func(key ClusterObjectKey) bool {
		return !filter(key)
	}
}
