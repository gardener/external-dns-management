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

package mappings

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

func ClusterName(name string) string {
	if name == CLUSTER_MAIN {
		return "<MAIN>"
	}
	return name
}

func DetermineClusters(cdefs cluster.Definitions, cmp Definition, names ...string) (utils.StringSet, []string, error) {
	clusters := utils.StringSet{}
	found := []string{}
	main_cluster := ""
	for i, name := range names {
		real, info := MapCluster(i == 0, name, cmp)
		rc := cdefs.Get(real)
		if rc == nil {
			if i == 0 {
				return nil, nil, fmt.Errorf("unknown cluster %s", info)
			}
			real = main_cluster
			info = fmt.Sprintf("%s (%s mapped to %s cluster)", found[0], name, ClusterName(CLUSTER_MAIN))
		}
		if i == 0 {
			main_cluster = real
		}
		found = append(found, info)
		clusters.Add(real)
	}
	return clusters, found, nil
}

func MapClusters(clusters cluster.Clusters, cmp Definition, names ...string) (cluster.Clusters, error) {
	mapped := cluster.NewClusters(clusters.Cache())
	var main_cluster cluster.Interface
	main_info := ""
	for i, name := range names {
		real, info := MapCluster(i == 0, name, cmp)
		cluster := clusters.GetCluster(real)
		if cluster == nil {
			if i == 0 {
				return nil, fmt.Errorf("cluster %s not found", cmp.MapInfo(name))
			}
			cluster = main_cluster
			info = fmt.Sprintf("%s (%s mapped to %s cluster)", main_info, name, ClusterName(CLUSTER_MAIN))
		}
		if i == 0 {
			main_cluster = cluster
			main_info = info
			mapped.Add(CLUSTER_MAIN, cluster, info)
		}
		mapped.Add(name, cluster, info)
	}
	return mapped, nil
}

func MapCluster(main bool, name string, cmp Definition) (mapped, info string) {
	if main {
		m := cmp.MapCluster(CLUSTER_MAIN)
		if m != CLUSTER_MAIN {
			return m, fmt.Sprintf("%s (mapped from %s=%q) as %s", m, ClusterName(CLUSTER_MAIN), name, ClusterName(CLUSTER_MAIN))
		}
		m = cmp.MapCluster(name)
		return m, fmt.Sprintf("%s as %s", cmp.MapInfo(name), ClusterName(CLUSTER_MAIN))
	}
	return cmp.MapCluster(name), cmp.MapInfo(name)
}
