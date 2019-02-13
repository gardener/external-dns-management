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

package reconcilers

import (
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
