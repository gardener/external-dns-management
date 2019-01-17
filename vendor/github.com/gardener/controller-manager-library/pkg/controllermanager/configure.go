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
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/groups"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
)

type Configuration struct {
	name           string
	description    string
	cluster_reg    cluster.Registry
	controller_reg controller.Registry
}

var _ cluster.RegistrationInterface = &Configuration{}
var _ mappings.RegistrationInterface = &Configuration{}
var _ groups.RegistrationInterface = &Configuration{}
var _ controller.RegistrationInterface = &Configuration{}

func Configure(name, desc string) Configuration {
	return Configuration{
		name:           name,
		description:    desc,
		cluster_reg:    cluster.NewRegistry(),
		controller_reg: controller.NewRegistry(),
	}
}

func (this Configuration) ByDefault() Configuration {
	this.cluster_reg = cluster.DefaultRegistry()
	this.controller_reg = controller.DefaultRegistry()
	return this
}

func (this Configuration) RegisterCluster(reg cluster.Registerable) error {
	return this.cluster_reg.RegisterCluster(reg)
}
func (this Configuration) MustRegisterCluster(reg cluster.Registerable) cluster.RegistrationInterface {
	return this.cluster_reg.MustRegisterCluster(reg)
}

func (this Configuration) RegisterMapping(reg mappings.Registerable) error {
	return this.controller_reg.RegisterMapping(reg)
}
func (this Configuration) MustRegisterMapping(reg mappings.Registerable) mappings.RegistrationInterface {
	return this.controller_reg.MustRegisterMapping(reg)
}

func (this Configuration) RegisterGroup(name string) (*groups.Configuration, error) {
	return this.controller_reg.RegisterGroup(name)
}
func (this Configuration) MustRegisterGroup(name string) *groups.Configuration {
	return this.controller_reg.MustRegisterGroup(name)
}

func (this Configuration) RegisterController(reg controller.Registerable, groups ...string) error {
	return this.controller_reg.RegisterController(reg, groups...)
}
func (this Configuration) MustRegisterController(reg controller.Registerable, groups ...string) controller.RegistrationInterface {
	return this.controller_reg.MustRegisterController(reg, groups...)
}

func (this Configuration) Definition() *Definition {
	return &Definition{
		name:            this.name,
		description:     this.description,
		cluster_defs:    this.cluster_reg.GetDefinitions(),
		controller_defs: this.controller_reg.GetDefinitions(),
	}
}
