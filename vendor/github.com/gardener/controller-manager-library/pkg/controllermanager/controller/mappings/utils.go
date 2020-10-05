/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package mappings

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
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
	s, _, infos, err := DetermineClusterMappings(cdefs, cmp, names...)
	return s, infos, err
}

func DetermineClusterMappings(cdefs cluster.Definitions, cmp Definition, names ...string) (utils.StringSet, controllermanager.Mapping, []string, error) {
	clusters := controllermanager.DefaultMapping{}
	eff := utils.StringSet{}
	found := []string{}
	main_cluster := ""
	for i, name := range names {
		real, info := MapCluster(i == 0, name, cmp)
		rc := cdefs.Get(real)
		if rc == nil {
			if i == 0 {
				return nil, nil, nil, fmt.Errorf("unknown cluster %s", info)
			}
			real = main_cluster
			info = fmt.Sprintf("%s (%s mapped to %s cluster)", found[0], name, ClusterName(CLUSTER_MAIN))
		}
		if i == 0 {
			main_cluster = real
		}
		found = append(found, info)
		eff.Add(real)
		clusters[name] = real
	}
	return eff, clusters, found, nil
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
