/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controllermanager

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	resources "github.com/gardener/controller-manager-library/pkg/resources/plain"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ConfigurationModifier func(c Configuration) Configuration

type Configuration struct {
	name          string
	description   string
	extension_reg extension.ExtensionRegistry
	cluster_reg   cluster.Registry
	configState

	globalMinimalWatch  resources.GroupKindSet
	clusterMinimalWatch map[string]resources.GroupKindSet
}

type configState struct {
	previous *configState
}

func (this *configState) pushState() {
	save := *this
	this.previous = &save
}

var _ cluster.RegistrationInterface = &Configuration{}

func Configure(name, desc string, scheme *runtime.Scheme) Configuration {
	return Configuration{
		name:          name,
		description:   desc,
		extension_reg: extension.NewExtensionRegistry(),
		cluster_reg:   cluster.NewRegistry(scheme),
		configState:   configState{},

		globalMinimalWatch:  resources.GroupKindSet{},
		clusterMinimalWatch: map[string]resources.GroupKindSet{},
	}
}

func (this Configuration) With(modifier ...ConfigurationModifier) Configuration {
	save := this.configState
	result := this
	for _, m := range modifier {
		result = m(result)
	}
	result.configState = save
	return result
}

func (this Configuration) Restore() Configuration {
	if &this.configState != nil {
		this.configState = *this.configState.previous
	}
	return this
}

func (this Configuration) ByDefault() Configuration {
	this.extension_reg = extension.DefaultRegistry()
	this.cluster_reg = cluster.DefaultRegistry()
	return this
}

func (this Configuration) GlobalMinimalWatch(groupKinds ...schema.GroupKind) Configuration {
	this.globalMinimalWatch.AddAll(groupKinds)
	return this
}

func (this Configuration) MinimalWatch(clusterName string, groupKinds ...schema.GroupKind) Configuration {
	m, ok := this.clusterMinimalWatch[clusterName]
	if !ok {
		m = resources.GroupKindSet{}
		this.clusterMinimalWatch[clusterName] = m
	}
	m.AddAll(groupKinds)
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

func (this Configuration) Definition() Definition {
	cluster_defs := this.cluster_reg.GetDefinitions().Reconfigure(func(configuration cluster.Configuration) cluster.Configuration {
		configuration = configuration.MininalWatches(this.globalMinimalWatch.AsArray()...)
		if m, ok := this.clusterMinimalWatch[configuration.Definition().Name()]; ok {
			configuration = configuration.MininalWatches(m.AsArray()...)
		}
		return configuration
	})
	return &_Definition{
		name:         this.name,
		description:  this.description,
		extensions:   this.extension_reg.GetDefinitions(),
		cluster_defs: cluster_defs,
	}
}
