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
	"github.com/gardener/controller-manager-library/pkg/utils"
	"sync"
)

///////////////////////////////////////////////////////////////////////////////
// cluster group Registrations
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
	definitions _Registrations
	controllers utils.StringSet
}

type _Registry struct {
	*_Definitions
}

var _ Definition = &_Definition{}
var _ Definitions = &_Definitions{}

func NewRegistry() Registry {
	registry := &_Registry{_Definitions: &_Definitions{definitions: _Registrations{}, controllers: utils.StringSet{}}}
	return registry
}

func DefaultDefinitions() Definitions {
	return registry.GetDefinitions()
}

func DefaultRegistry() Registry {
	return registry
}

////////////////////////////////////////////////////////////////////////////////

var _ Registry = &_Registry{}

func (this *_Registry) RegisterGroup(name string) (*Configuration, error) {
	this.lock.Lock()
	defer this.lock.Unlock()

	def := this._Definitions.definitions[name]
	if def == nil {
		if this.controllers.Contains(name) {
			return nil, fmt.Errorf("name %q already busy by configured controller with this name", name)
		}
		def = &_Definition{name: name, controllers: utils.StringSet{}}
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
	return &_Definitions{definitions: defs, controllers: this.controllers.Copy()}
}

func (this *_Definitions) Get(name string) Definition {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.definitions[name]
}

func (this *_Definition) Definition() Definition {
	return this
}

///////////////////////////////////////////////////////////////////////////////

var registry = NewRegistry()

type Configuration struct {
	registry   *_Registry
	definition *_Definition
}

func (this Configuration) Controllers(names ...string) error {
	for _, n := range names {
		if this.registry.definitions[n] != nil {
			panic(fmt.Sprintf("controller name %q already used as group name", n))
		}
		this.definition.controllers.Add(n)
		this.registry.controllers.Add(n)
	}

	return nil
}

///////////////////////////////////////////////////////////////////////////////

func Register(name string) (*Configuration, error) {
	return registry.RegisterGroup(name)
}

func MustRegister(name string) *Configuration {
	return registry.MustRegisterGroup(name)
}
