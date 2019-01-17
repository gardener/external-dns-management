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

package controller

import (
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/groups"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type Definitions interface {
	Get(name string) Definition
	Size() int
	Names() utils.StringSet
	Groups() groups.Definitions
	GetMappingsFor(name string) (mappings.Definition, error)
	DetermineRequestedClusters(clusters cluster.Definitions, sets ...utils.StringSet) (utils.StringSet, error)
	Registrations(names ...string) (Registrations, error)
	ExtendConfig(cfg *config.Config)
}

func (this *_Definitions) Size() int {
	return len(this.definitions)
}

func (this *_Definitions) Groups() groups.Definitions {
	return this.groups
}

func (this *_Definitions) Names() utils.StringSet {
	set := utils.StringSet{}
	for n := range this.definitions {
		set.Add(n)
	}
	return set
}

func (this *_Definitions) GetMappingsFor(name string) (mappings.Definition, error) {
	return this.mappings.GetEffective(name, this.groups)
}

func (this *_Definitions) DetermineRequestedClusters(cdefs cluster.Definitions, controllersets ...utils.StringSet) (utils.StringSet, error) {
	var controller_names utils.StringSet
	switch len(controllersets) {
	case 0:
		controller_names = this.definitions.Names()
	case 1:
		controller_names = controllersets[0]
	default:
		controller_names = utils.NewStringSetBySets(controllersets...)
	}
	this.lock.RLock()
	defer this.lock.RUnlock()

	clusters := utils.StringSet{}
	logger.Infof("determining required clusters:")
	for n := range controller_names {
		def := this.definitions[n]
		if def == nil {
			return nil, fmt.Errorf("controller %q not definied", n)
		}
		names := cluster.Canonical(def.RequiredClusters())
		cmp, err := this.GetMappingsFor(def.GetName())
		if err != nil {
			return nil, err
		}
		logger.Infof("  for controller %s:", n)
		logger.Infof("     logical clusters %s", n, utils.Strings(names...))

		set, found, err := mappings.DetermineClusters(cdefs, cmp, names...)
		if err != nil {
			return nil, fmt.Errorf("controller %q %s", def.GetName(), err)
		}
		clusters.AddSet(set)
		logger.Infof("  mapped to %s", utils.Strings(found...))
	}
	return clusters, nil
}

func (this *_Definitions) Registrations(names ...string) (Registrations, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	var r = Registrations{}

	if len(names) == 0 {
		r = this.definitions.Copy()
	} else {
		for _, name := range names {
			def := this.definitions[name]
			if def == nil {
				return nil, fmt.Errorf("controller %q not found", name)
			}
			r[name] = def
		}
	}

	return r, nil
}
