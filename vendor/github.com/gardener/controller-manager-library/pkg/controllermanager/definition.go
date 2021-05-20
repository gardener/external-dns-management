/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controllermanager

import (
	"github.com/gardener/controller-manager-library/pkg/configmain"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Definition interface {
	GetName() string
	GetDescription() string
	GetExtensions() extension.ExtensionDefinitions
	ExtensionDefinition(name string) extension.Definition
	ClusterDefinitions() cluster.Definitions
	ExtendConfig(cfg *configmain.Config)
	GroupKindMigrations() []schema.GroupKind
}

type _Definition struct {
	name         string
	description  string
	extensions   extension.ExtensionDefinitions
	cluster_defs cluster.Definitions
	gkMigrations []schema.GroupKind
}

func (this *_Definition) GetName() string {
	return this.name
}

func (this *_Definition) GetDescription() string {
	return this.description
}

func (this *_Definition) GetExtensions() extension.ExtensionDefinitions {
	defs := extension.ExtensionDefinitions{}
	for n, e := range this.extensions {
		defs[n] = e
	}
	return defs
}

func (this *_Definition) ExtensionDefinition(name string) extension.Definition {
	for _, e := range this.extensions {
		if e.Name() == name {
			return e
		}
	}
	return nil
}

func (this *_Definition) ClusterDefinitions() cluster.Definitions {
	return this.cluster_defs
}

func (this *_Definition) ExtendConfig(cfg *configmain.Config) {
	ccfg := areacfg.NewConfig(this.GetName())
	cfg.AddSource(areacfg.OPTION_SOURCE, ccfg)

	for _, e := range this.extensions {
		e.ExtendConfig(ccfg)
	}
	this.cluster_defs.ExtendConfig(ccfg)
}

func (this *_Definition) GroupKindMigrations() []schema.GroupKind {
	return this.gkMigrations
}

func DefaultDefinition(name, desc string) Definition {
	return Configure(name, desc, nil).ByDefault().Definition()
}
