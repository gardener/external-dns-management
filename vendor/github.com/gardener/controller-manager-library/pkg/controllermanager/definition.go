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
	"github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/groups"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
)

type Definition struct {
	name            string
	description     string
	cluster_defs    cluster.Definitions
	controller_defs controller.Definitions
}

func (this *Definition) GetName() string {
	return this.name
}

func (this *Definition) GetDescription() string {
	return this.description
}

func (this *Definition) ClusterDefinitions() cluster.Definitions {
	return this.cluster_defs
}

func (this *Definition) ControllerDefinitions() controller.Definitions {
	return this.controller_defs
}

func (this *Definition) Groups() groups.Definitions {
	return this.controller_defs.Groups()
}

func (this *Definition) Registrations(names ...string) (controller.Registrations, error) {
	return this.controller_defs.Registrations(names...)
}

func (this *Definition) GetMappingsFor(name string) (mappings.Definition, error) {
	return this.controller_defs.GetMappingsFor(name)
}

func (this *Definition) ExtendConfig(cfg *config.Config) {
	this.cluster_defs.ExtendConfig(cfg)
	this.controller_defs.ExtendConfig(cfg)
}

func DefaultDefinition(name, desc string) *Definition {
	return Configure(name, desc).ByDefault().Definition()
}
