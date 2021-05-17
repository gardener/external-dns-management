/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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
	elemType    string
	definitions map[string]definitions
}

type _Registry struct {
	*_Definitions
}

var _ Definition = &DefinitionImpl{}
var _ Definitions = &_Definitions{}

func NewRegistry() Registry {
	registry := &_Registry{_Definitions: &_Definitions{definitions: map[string]definitions{}}}
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

var identity = NewDefinition("", "<identity>")

func (this *_Registry) GetDefinitions() Definitions {
	defs := map[string]definitions{}
	for k, v := range this.definitions {
		defs[k] = v.Copy()
	}
	return &_Definitions{definitions: defs}
}

///////////////////////////////////////////////////////////////////////////////

func newDefinitions(def *DefinitionImpl) *_Definitions {
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

func (this *DefinitionImpl) Definition() Definition {
	return this
}
