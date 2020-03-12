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
	"github.com/gardener/controller-manager-library/pkg/configmain"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
)

type Definition struct {
	name         string
	description  string
	extensions   extension.ExtensionDefinitions
	cluster_defs cluster.Definitions
}

func (this *Definition) GetName() string {
	return this.name
}

func (this *Definition) GetDescription() string {
	return this.description
}

func (this *Definition) GetExtensions() extension.ExtensionDefinitions {
	defs := extension.ExtensionDefinitions{}
	for n, e := range this.extensions {
		defs[n] = e
	}
	return defs
}

func (this *Definition) ExtensionDefinition(name string) extension.Definition {
	for _, e := range this.extensions {
		if e.Name() == name {
			return e
		}
	}
	return nil
}

func (this *Definition) ClusterDefinitions() cluster.Definitions {
	return this.cluster_defs
}

func (this *Definition) ExtendConfig(cfg *configmain.Config) {
	ccfg := areacfg.NewConfig()
	cfg.AddSource(areacfg.OPTION_SOURCE, ccfg)

	for _, e := range this.extensions {
		e.ExtendConfig(ccfg)
	}
	this.cluster_defs.ExtendConfig(ccfg)
}

func DefaultDefinition(name, desc string) *Definition {
	return Configure(name, desc, nil).ByDefault().Definition()
}
