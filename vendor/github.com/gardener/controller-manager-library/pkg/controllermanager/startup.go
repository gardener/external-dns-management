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

package controllermanager

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
)

type StartupGroup interface {
	Startup() error
	Add(c Controller)
}

type startupgroup struct {
	manager     *ControllerManager
	cluster     cluster.Interface
	controllers []Controller
}

func (this *startupgroup) Add(c Controller) {
	this.controllers = append(this.controllers, c)
}

func (this *startupgroup) Startup() error {
	for _, c := range this.controllers {
		err := this.manager.startController(c)
		if err != nil {
			return err
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

func (c *ControllerManager) getPlainStartupGroup(cluster cluster.Interface) StartupGroup {
	g := c.plain_groups[cluster.GetName()]
	if g == nil {
		g = &startupgroup{c, cluster, nil}
		c.plain_groups[cluster.GetName()] = g
	}
	return g
}

func (c *ControllerManager) getLeaseStartupGroup(cluster cluster.Interface) StartupGroup {
	g := c.lease_groups[cluster.GetName()]
	if g == nil {
		g = &leasestartupgroup{startupgroup{c, cluster, nil}}
		c.lease_groups[cluster.GetName()] = g
	}
	return g
}

func (c *ControllerManager) startGroups(grps ...map[string]StartupGroup) error {
	for _, grp := range grps {
		for _, g := range grp {
			err := g.Startup()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
