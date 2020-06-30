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

package cluster

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

///////////////////////////////////////////////////////////////////////////////
// cluster definitions
///////////////////////////////////////////////////////////////////////////////

type Registrations map[string]Definition

type Registerable interface {
	Definition() Definition
}

type RegistrationInterface interface {
	RegisterCluster(Registerable) error
	MustRegisterCluster(Registerable) RegistrationInterface
}

type Registry interface {
	RegistrationInterface
	GetDefinitions() Definitions
}

type _Definitions struct {
	lock        sync.RWMutex
	definitions Registrations
	scheme      *runtime.Scheme
}

type _Registry struct {
	*_Definitions
}

var _ Definition = &_Definition{}
var _ Definitions = &_Definitions{}

func NewRegistry(scheme *runtime.Scheme) Registry {
	if scheme == nil {
		scheme = resources.DefaultScheme()
	}
	registry := &_Registry{_Definitions: &_Definitions{definitions: Registrations{}, scheme: scheme}}
	Configure(DEFAULT, "kubeconfig", "default cluster access").MustRegisterAt(registry)
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

func (this *_Registry) RegisterCluster(reg Registerable) error {
	def := reg.Definition()
	if def == nil {
		return fmt.Errorf("no definition found")
	}
	this.lock.Lock()
	defer this.lock.Unlock()

	if old := this.definitions[def.Name()]; old != nil {
		msg := fmt.Sprintf("cluster request for %q")
		new := copy(old)
		err := utils.FillStringValue(msg, &new.configOptionName, def.ConfigOptionName())
		if err != nil {
			return err
		}
		err = utils.FillStringValue(msg, &new.description, def.Description())
		if err != nil {
			return err
		}
		def = new
	}
	this.definitions[def.Name()] = def
	return nil
}

func (this *_Registry) MustRegisterCluster(reg Registerable) RegistrationInterface {
	err := this.RegisterCluster(reg)
	if err != nil {
		panic(err)
	}
	return this
}

////////////////////////////////////////////////////////////////////////////////

func (this *_Registry) GetDefinitions() Definitions {
	defs := Registrations{}
	for k, v := range this.definitions {
		defs[k] = v
	}
	return &_Definitions{definitions: defs}
}

func (this *_Definitions) Get(name string) Definition {
	this.lock.RLock()
	defer this.lock.RUnlock()
	if c, ok := this.definitions[name]; ok {
		return c
	}
	return nil
}

func (this *_Definitions) GetScheme() *runtime.Scheme {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.scheme
}

///////////////////////////////////////////////////////////////////////////////

var registry = NewRegistry(nil)

type Configuration struct {
	definition _Definition
}

var _ Registerable = Configuration{}

func Configure(name string, option string, short string) Configuration {
	return Configuration{_Definition{name, "", option, short, nil}}
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

///////////////////////////////////////////////////////////////////////////////

func Register(name string, option string, short string) error {
	return Configure(name, option, short).Register()
}

func MustRegister(name string, option string, short string) {
	Configure(name, option, short).MustRegister()
}
