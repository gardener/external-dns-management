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

package reconcilers

import (
	v1 "k8s.io/api/core/v1"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

////////////////////////////////////////////////////////////////////////////////

var secretGK = resources.NewGroupKind("core", "Secret")

func SecretUsageReconciler(clusterName string) controller.ConfigurationModifier {
	return UsageReconcilerForGKs("secrets", clusterName, secretGK)
}

////////////////////////////////////////////////////////////////////////////////

type SecretUsageCache struct {
	*SimpleUsageCache
	controller controller.Interface
}

func AccessSecretUsageCache(controller controller.Interface) *SecretUsageCache {
	return &SecretUsageCache{
		SimpleUsageCache: GetSharedSimpleUsageCache(controller),
		controller:       controller,
	}
}
func (this *SecretUsageCache) getCluster(src ...cluster.Interface) cluster.Interface {
	if len(src) == 0 || src[0] == nil {
		return this.controller.GetMainCluster()
	} else {
		return src[0]
	}
}

func (this *SecretUsageCache) GetSecretDataByName(name resources.ObjectName, src ...cluster.Interface) (map[string][]byte, error) {
	cluster := this.getCluster(src...)
	resc, err := cluster.GetResource(secretGK)
	obj, err := resc.GetCached(name)
	if err != nil {
		return nil, err
	}
	return obj.Data().(*v1.Secret).Data, nil
}

func (this *SecretUsageCache) GetUsersForSecretByName(name resources.ObjectName, src ...cluster.Interface) resources.ClusterObjectKeySet {
	cluster := this.getCluster(src...)
	key := resources.NewClusterKey(cluster.GetId(), secretGK, name.Namespace(), name.Name())
	return this.GetUsersFor(key)
}
