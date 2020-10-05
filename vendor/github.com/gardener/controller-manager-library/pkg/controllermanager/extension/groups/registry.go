/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package groups

import (
	"fmt"
	"sync"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

///////////////////////////////////////////////////////////////////////////////
// Group Registrations
///////////////////////////////////////////////////////////////////////////////

type Registrations map[string]Definition
type _Registrations map[string]*_Definition

type RegistrationInterface interface {
	RegisterGroup(name string) (*Configuration, error)
	MustRegisterGroup(name string) *Configuration
}

type Registry interface {
	RegistrationInterface
	GetDefinitions() Definitions
}

type _Definitions struct {
	lock        sync.RWMutex
	typeName    string
	definitions _Registrations
	elements    utils.StringSet
}

type _Registry struct {
	*_Definitions
}

var _ Definition = &_Definition{}
var _ Definitions = &_Definitions{}

func NewRegistry(elementType string) Registry {
	registry := &_Registry{
		_Definitions: &_Definitions{
			typeName:    elementType,
			definitions: _Registrations{},
			elements:    utils.StringSet{},
		},
	}
	return registry
}

////////////////////////////////////////////////////////////////////////////////

var _ Registry = &_Registry{}

func (this *_Registry) RegisterGroup(name string) (*Configuration, error) {
	this.lock.Lock()
	defer this.lock.Unlock()

	def := this._Definitions.definitions[name]
	if def == nil {
		if this.elements.Contains(name) {
			return nil, fmt.Errorf("name %q already busy by configured %s with this name", name, this.typeName)
		}
		def = &_Definition{name: name, members: utils.StringSet{}, explicit: utils.StringSet{}}
		this._Definitions.definitions[name] = def
	}
	return &Configuration{this, def}, nil
}

func (this *_Registry) MustRegisterGroup(name string) *Configuration {
	cfg, err := this.RegisterGroup(name)
	if err != nil {
		panic(err)
	}
	return cfg
}

////////////////////////////////////////////////////////////////////////////////

func (this *_Registry) GetDefinitions() Definitions {
	defs := _Registrations{}
	for k, v := range this.definitions {
		defs[k] = v.copy()
	}
	return &_Definitions{
		typeName:    this.typeName,
		definitions: defs,
		elements:    this.elements.Copy(),
	}
}

func (this *_Definitions) Get(name string) Definition {
	this.lock.RLock()
	defer this.lock.RUnlock()
	if g, ok := this.definitions[name]; ok {
		return g
	}
	return nil
}

func (this *_Definition) Definition() Definition {
	return this
}

///////////////////////////////////////////////////////////////////////////////

type Configuration struct {
	registry   *_Registry
	definition *_Definition
}

func (this Configuration) Members(names ...string) error {
	for _, n := range names {
		if this.registry.definitions[n] != nil {
			panic(fmt.Sprintf("%s name %q already used as group name", this.registry.typeName, n))
		}
		this.definition.members.Add(n)
		this.registry.elements.Add(n)
	}

	return nil
}

func (this Configuration) ActivateExplicitly(names ...string) {
	this.definition.explicit.AddAll(names)
}
