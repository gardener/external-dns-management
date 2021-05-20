/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cluster

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources"
)

type Configuration struct {
	definition _Definition
}

var _ Registerable = Configuration{}

func Configure(name string, option string, short string) Configuration {
	return Configuration{
		_Definition{
			name:             name,
			fallback:         "",
			configOptionName: option,
			description:      short,
			scheme:           nil,
			minimalWatches:   resources.NewGroupKindSet(),
		},
	}
}

func (this Configuration) Fallback(name string) Configuration {
	this.definition.fallback = name
	return this
}

func (this Configuration) Scheme(scheme *runtime.Scheme) Configuration {
	this.definition.scheme = scheme
	return this
}

func (this Configuration) Definition() Definition {
	return &this.definition
}

func (this Configuration) Register() error {
	return registry.RegisterCluster(this)
}

func (this Configuration) MustRegister() Configuration {
	registry.MustRegisterCluster(this)
	return this
}

func (this Configuration) RegisterAt(registry Registry) error {
	return registry.RegisterCluster(this)
}

func (this Configuration) MustRegisterAt(registry Registry) Configuration {
	registry.MustRegisterCluster(this)
	return this
}

func (this Configuration) MininalWatches(gk ...schema.GroupKind) Configuration {
	this.definition.minimalWatches.AddAll(gk)
	return this
}

////////////////////////////////////////////////////////////////////////////////

var registry = NewRegistry(nil)

func DefaultDefinitions() Definitions {
	return registry.GetDefinitions()
}

func DefaultRegistry() Registry {
	return registry
}

///////////////////////////////////////////////////////////////////////////////

func Register(name string, option string, short string) error {
	return Configure(name, option, short).Register()
}

func MustRegister(name string, option string, short string) {
	Configure(name, option, short).MustRegister()
}
