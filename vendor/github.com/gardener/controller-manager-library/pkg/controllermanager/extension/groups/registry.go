/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved.
 * This file is licensed under the Apache Software License, v. 2 except as noted
 * otherwise in the LICENSE file
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
