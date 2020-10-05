/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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
