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

package groups

import (
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

const DEFAULT = "default"

type Definitions interface {
	Get(name string) Definition
	Activate(controllers []string) (utils.StringSet, error)
	AllGroups() map[string]utils.StringSet
	AllControllers() utils.StringSet
	AllActivateExplicitlyControllers() utils.StringSet
}

type Definition interface {
	Controllers() utils.StringSet
	ActivateExplicitlyControllers() utils.StringSet
}

type _Definition struct {
	name        string
	controllers utils.StringSet

	activateExplicitylyControllers utils.StringSet
}

func (this *_Definition) copy() *_Definition {
	return &_Definition{name: this.name, controllers: this.controllers.Copy(),
		activateExplicitylyControllers: this.activateExplicitylyControllers.Copy()}
}

func (this *_Definition) Controllers() utils.StringSet {
	return this.controllers.Copy()
}

func (this *_Definition) ActivateExplicitlyControllers() utils.StringSet {
	return this.activateExplicitylyControllers.Copy()
}

////////////////////////////////////////////////////////////////////////////////

func (this *_Definitions) Activate(controllers []string) (utils.StringSet, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	active := utils.StringSet{}
	explicitActive := utils.StringSet{}
	if len(controllers) == 0 {
		logger.Infof("activating all controllers")
		active = this.AllControllers()
	} else {
		for _, name := range controllers {
			g := this.definitions[name]
			if g != nil {
				logger.Infof("activating controller group %q", name)
				active.AddSet(g.controllers)
			} else {
				if name == "all" {
					logger.Infof("activating all controllers")
					for _, g := range this.definitions {
						active.AddSet(g.controllers)
					}
				} else {
					if this.controllers.Contains(name) {
						logger.Infof("activating controller %q", name)
						active.Add(name)
						explicitActive.Add(name)
					} else {
						return nil, fmt.Errorf("unknown controller or group %q", name)
					}
				}
			}
		}
	}
	toBeActivatedExplicitly := this.AllActivateExplicitlyControllers()
	for name := range active {
		if !explicitActive.Contains(name) && toBeActivatedExplicitly.Contains(name) {
			active.Remove(name)
		}
	}

	logger.Infof("activated controllers: %s", active)
	return active, nil
}

func (this *_Definitions) AllGroups() map[string]utils.StringSet {
	this.lock.RLock()
	defer this.lock.RUnlock()
	active := map[string]utils.StringSet{}
	for n, g := range this.definitions {
		active[n] = g.controllers.Copy()
	}
	return active
}

func (this *_Definitions) AllControllers() utils.StringSet {
	this.lock.RLock()
	defer this.lock.RUnlock()
	active := utils.StringSet{}
	for _, g := range this.definitions {
		active.AddSet(g.controllers)
	}
	return active
}

func (this *_Definitions) AllActivateExplicitlyControllers() utils.StringSet {
	this.lock.RLock()
	defer this.lock.RUnlock()
	set := utils.StringSet{}
	for _, g := range this.definitions {
		set.AddSet(g.activateExplicitylyControllers)
	}
	return set
}
