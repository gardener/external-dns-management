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

package mappings

import (
	"fmt"
	"sync"
)

///////////////////////////////////////////////////////////////////////////////
// cluster name mapping definitions
///////////////////////////////////////////////////////////////////////////////

type definitions map[string]Definition

func (this definitions) String() string {
	return fmt.Sprintf("%v", map[string]Definition(this))[3:]
}

func (this definitions) Copy() definitions {
	new := definitions{}
	for k, v := range this {
		new[k] = v
	}
	return new
}

type Registerable interface {
	Definition() Definition
}

type RegistrationInterface interface {
	RegisterMapping(Registerable) error
	MustRegisterMapping(Registerable) RegistrationInterface
}

type Registry interface {
	RegistrationInterface
	GetDefinitions() Definitions
}

type _Definitions struct {
	lock        sync.RWMutex
	definitions map[string]definitions
}

type _Registry struct {
	*_Definitions
}

var _ Definition = &_Definition{}
var _ Definitions = &_Definitions{}

func NewRegistry() Registry {
	registry := &_Registry{_Definitions: &_Definitions{definitions: map[string]definitions{}}}
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

func (this *_Registry) RegisterMapping(reg Registerable) error {
	def := reg.Definition()
	if def == nil {
		return fmt.Errorf("no definition found")
	}
	this.lock.Lock()
	defer this.lock.Unlock()

	defs := this._Definitions.getForType(def.Type())
	if old := defs[def.Name()]; old != nil {
		return fmt.Errorf("mapping for %s %q already defined", def.Type(), def.Name())
	} else {
		defs[def.Name()] = def
	}
	return nil
}

func (this *_Registry) MustRegisterMapping(reg Registerable) RegistrationInterface {
	err := this.RegisterMapping(reg)
	if err != nil {
		panic(err)
	}
	return this
}

////////////////////////////////////////////////////////////////////////////////

var identity = newDefinition("", "<identity>")

func (this *_Registry) GetDefinitions() Definitions {
	defs := map[string]definitions{}
	for k, v := range this.definitions {
		defs[k] = v.Copy()
	}
	return &_Definitions{definitions: defs}
}

///////////////////////////////////////////////////////////////////////////////

func newDefinitions(def *_Definition) *_Definitions {
	return &_Definitions{definitions: map[string]definitions{def.Type(): {def.Name(): def}}}
}

func (this *_Definitions) String() string {
	return fmt.Sprintf("%v", this.definitions)[3:]
}

func (this *_Definitions) Get(mtype, name string) Definition {
	this.lock.RLock()
	defer this.lock.RUnlock()
	d, ok := this.definitions[mtype][name]
	if !ok {
		return identity
	}
	return d
}

func (this *_Definitions) getForType(t string) definitions {
	defs := this.definitions[t]
	if defs == nil {
		defs = map[string]Definition{}
		this.definitions[t] = defs
	}
	return defs
}

///////////////////////////////////////////////////////////////////////////////

func (this *_Definition) Definition() Definition {
	return this
}

///////////////////////////////////////////////////////////////////////////////

var registry = NewRegistry()

type empty struct{}

func Configure() empty {
	return empty{}
}

func ForController(name string) Configuration {
	return Configuration{*newDefinitionForController(name)}
}

func ForControllerGroup(name string) Configuration {
	return Configuration{*newDefinitionForGroup(name)}
}

func (this empty) ForController(name string) Configuration {
	return ForController(name)
}

func (this empty) ForControllerGroup(name string) Configuration {
	return ForControllerGroup(name)
}

type Configuration struct {
	definition _Definition
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
	new := map[string]string{}
	for k, v := range this.definition.mappings {
		new[k] = v
	}
	this.definition.mappings = new
}

func (this Configuration) Map(cluster, to string) Configuration {
	this.copy()
	this.definition.mappings[cluster] = to
	return this
}

///////////////////////////////////////////////////////////////////////////////

func Register(reg Registerable) error {
	return registry.RegisterMapping(reg)
}

func MustRegister(reg Registerable) RegistrationInterface {
	return registry.MustRegisterMapping(reg)
}
