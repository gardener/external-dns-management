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
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"

	"k8s.io/apimachinery/pkg/runtime"
)

type Configuration struct {
	name          string
	description   string
	extension_reg extension.ExtensionRegistry
	cluster_reg   cluster.Registry
}

var _ cluster.RegistrationInterface = &Configuration{}

func Configure(name, desc string, scheme *runtime.Scheme) Configuration {
	return Configuration{
		name:          name,
		description:   desc,
		extension_reg: extension.NewExtensionRegistry(),
		cluster_reg:   cluster.NewRegistry(scheme),
	}
}

func (this Configuration) ByDefault() Configuration {
	this.extension_reg = extension.DefaultRegistry()
	this.cluster_reg = cluster.DefaultRegistry()
	return this
}

func (this Configuration) RegisterExtension(reg extension.ExtensionType) {
	this.extension_reg.RegisterExtension(reg)
}
func (this Configuration) Extension(name string) extension.ExtensionType {
	for _, e := range this.extension_reg.GetExtensionTypes() {
		if e.Name() == name {
			return e
		}
	}
	return nil
}
func (this Configuration) RegisterCluster(reg cluster.Registerable) error {
	return this.cluster_reg.RegisterCluster(reg)
}
func (this Configuration) MustRegisterCluster(reg cluster.Registerable) cluster.RegistrationInterface {
	return this.cluster_reg.MustRegisterCluster(reg)
}

func (this Configuration) Definition() *Definition {
	return &Definition{
		name:         this.name,
		description:  this.description,
		extensions:   this.extension_reg.GetDefinitions(),
		cluster_defs: this.cluster_reg.GetDefinitions(),
	}
}
