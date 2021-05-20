/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package mappings

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension/mappings"
)

func DefaultDefinitions() Definitions {
	return registry.GetDefinitions()
}

func DefaultRegistry() Registry {
	return registry
}

////////////////////////////////////////////////////////////////////////////////

var registry = NewRegistry()

type empty struct{}

func Configure() empty {
	return empty{}
}

func ForController(name string) Configuration {
	return Configuration{*mappings.NewDefinition(TYPE_CONTROLLER, name)}
}

func ForControllerGroup(name string) Configuration {
	return Configuration{*mappings.NewDefinition(TYPE_GROUP, name)}
}

func (this empty) ForController(name string) Configuration {
	return ForController(name)
}

func (this empty) ForControllerGroup(name string) Configuration {
	return ForControllerGroup(name)
}

type Configuration struct {
	definition mappings.DefinitionImpl
}

func (this Configuration) Definition() Definition {
	return &this.definition
}

func (this Configuration) Register() error {
	return registry.RegisterMapping(this)
}

func (this Configuration) MustRegister() Configuration {
	registry.MustRegisterMapping(this)
	return this
}

func (this Configuration) RegisterAt(registry Registry) error {
	return registry.RegisterMapping(this)
}

func (this Configuration) MustRegisterAt(registry Registry) Configuration {
	registry.MustRegisterMapping(this)
	return this
}

///////////////////////////////////////////////////////////////////////////////

func (this *Configuration) copy() {
	this.definition.Copy()
}

func (this Configuration) Map(cluster, to string) Configuration {
	this.copy()
	this.definition.SetMapping(cluster, to)
	return this
}

///////////////////////////////////////////////////////////////////////////////

func Register(reg Registerable) error {
	return registry.RegisterMapping(reg)
}

func MustRegister(reg Registerable) RegistrationInterface {
	return registry.MustRegisterMapping(reg)
}
