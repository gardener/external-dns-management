/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package reconcilers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

func ClusterResources(cluster string, gks ...schema.GroupKind) Resources {
	return func(c controller.Interface) []resources.Interface {
		result := []resources.Interface{}
		resources := c.GetCluster(cluster).Resources()
		for _, gk := range gks {
			res, err := resources.Get(gk)
			if err != nil {
				panic(fmt.Errorf("resources type %s not found: %s", gk, err))
			}
			result = append(result, res)
		}
		return result
	}
}

func MainResources(gks ...schema.GroupKind) Resources {
	return ClusterResources("", gks...)
}
