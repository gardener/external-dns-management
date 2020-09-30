/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"fmt"
	"sync"

	"github.com/gardener/controller-manager-library/pkg/config"
	cgroups "github.com/gardener/controller-manager-library/pkg/controllermanager/controller/groups"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension/groups"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

///////////////////////////////////////////////////////////////////////////////
// controller Registrations
///////////////////////////////////////////////////////////////////////////////

type Registrations map[string]Definition

func (this Registrations) Copy() Registrations {
	r := Registrations{}
	for n, def := range this {
		r[n] = def
	}
	return r
}

func (this Registrations) Names() utils.StringSet {
	r := utils.StringSet{}
	for n := range this {
		r.Add(n)
	}
	return r
}

type Registerable interface {
	Definition() Definition
}

type RegistrationInterface interface {
	RegisterController(Registerable, ...string) error
	MustRegisterController(Registerable, ...string) RegistrationInterface
}

type Registry interface {
	RegistrationInterface
	mappings.RegistrationInterface
	groups.RegistrationInterface
	GetDefinitions() *_Definitions
}

type _Definitions struct {
	lock        sync.RWMutex
	definitions Registrations
	mappings    mappings.Definitions
	groups      groups.Definitions

	shared map[string]*config.ArbitraryOption
}

type _Registry struct {
	*_Definitions
	mappings mappings.Registry
	groups   groups.Registry
}

var _ Definition = &_Definition{}
var _ Definitions = &_Definitions{}

func NewRegistry() Registry {
	return newRegistry(mappings.NewRegistry(), cgroups.NewRegistry())
}

func newRegistry(mappings mappings.Registry, groups groups.Registry) Registry {
	return &_Registry{_Definitions: &_Definitions{definitions: Registrations{}}, mappings: mappings, groups: groups}
}

func DefaultDefinitions() Definitions {
	return registry.GetDefinitions()
}

func DefaultRegistry() Registry {
	return registry
}

////////////////////////////////////////////////////////////////////////////////

var _ Registry = &_Registry{}

func (this *_Registry) RegisterController(reg Registerable, group ...string) error {
	def := reg.Definition()
	if def == nil {
		return fmt.Errorf("no _Definition found")
	}
	this.lock.Lock()
	defer this.lock.Unlock()

	if def.MainResource() == nil {
		return fmt.Errorf("no main resource for controller %q", def.Name())
	}
	if d, ok := this.definitions[def.Name()]; ok && d != def {
		return fmt.Errorf("multiple registration of controller %q", def.Name())
	}
	logger.Infof("Registering controller %s", def.Name())

	if len(group) == 0 {
		err := this.addToGroup(def, groups.DEFAULT)
		if err != nil {
			return err
		}
	} else {
		for _, g := range group {
			err := this.addToGroup(def, g)
			if err != nil {
				return err
			}
		}
	}
	this.definitions[def.Name()] = def
	return nil
}

func (this *_Registry) MustRegisterController(reg Registerable, groups ...string) RegistrationInterface {
	err := this.RegisterController(reg, groups...)
	if err != nil {
		panic(err)
	}
	return this
}

func (this *_Registry) RegisterMapping(reg mappings.Registerable) error {
	return this.mappings.RegisterMapping(reg)
}
func (this *_Registry) RegisterGroup(name string) (*groups.Configuration, error) {
	return this.groups.RegisterGroup(name)
}

func (this *_Registry) MustRegisterMapping(reg mappings.Registerable) mappings.RegistrationInterface {
	return this.mappings.MustRegisterMapping(reg)
}
func (this *_Registry) MustRegisterGroup(name string) *groups.Configuration {
	return this.groups.MustRegisterGroup(name)
}

////////////////////////////////////////////////////////////////////////////////

func (this *_Registry) GetDefinitions() *_Definitions {
	defs := Registrations{}
	for k, v := range this.definitions {
		defs[k] = v
	}
	return &_Definitions{
		definitions: defs,
		groups:      this.groups.GetDefinitions(),
		mappings:    this.mappings.GetDefinitions(),
		shared:      map[string]*config.ArbitraryOption{},
	}
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

var registry = newRegistry(mappings.DefaultRegistry(), cgroups.DefaultRegistry())

func (this *_Registry) addToGroup(def Definition, name string) error {
	grp, err := this.groups.RegisterGroup(name)
	if err != nil {
		return err
	}
	if def.ActivateExplicitly() {
		grp.ActivateExplicitly(def.Name())
	}
	return grp.Members(def.Name())
}

///////////////////////////////////////////////////////////////////////////////

func Register(reg Registerable, groups ...string) error {
	return registry.RegisterController(reg, groups...)
}

func MustRegister(reg Registerable, groups ...string) RegistrationInterface {
	return registry.MustRegisterController(reg, groups...)
}
