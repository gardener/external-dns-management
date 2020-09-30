/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package resources

import (
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type DefaultClusterIdMigration struct {
	migrations map[string]string
}

var _ ClusterIdMigration = &DefaultClusterIdMigration{}
var _ ClusterIdMigrationProvider = &DefaultClusterIdMigration{}

func ClusterIdMigrationFor(clusters ...Cluster) ClusterIdMigration {
	migrations := map[string]string{}
	for _, c := range clusters {
		id := c.GetId()
		for o := range c.GetMigrationIds() {
			migrations[o] = id
		}
	}
	if len(migrations) == 0 {
		return nil
	}
	return &DefaultClusterIdMigration{migrations}
}

func (this *DefaultClusterIdMigration) RequireMigration(id string) string {
	if new, ok := this.migrations[id]; ok {
		return new
	}
	return ""
}

func (this *DefaultClusterIdMigration) GetClusterIdMigration() ClusterIdMigration {
	return this
}

func (this *DefaultClusterIdMigration) String() string {
	m := map[string]utils.StringSet{}

	for k, v := range this.migrations {
		s := m[v]
		if s == nil {
			s = utils.StringSet{}
			m[v] = s
		}
		s.Add(k)
	}

	sep := ""
	r := ""
	for k, v := range m {
		r = r + sep + k + " <- " + v.String()
		sep = ", "
	}
	return r
}
