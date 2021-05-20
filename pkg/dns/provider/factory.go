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

package provider

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/utils"

	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type DNSHandlerCreatorFunction func(config *DNSHandlerConfig) (DNSHandler, error)

type Factory struct {
	typecode              string
	create                DNSHandlerCreatorFunction
	optionCreator         extension.OptionSourceCreator
	genericDefaults       *GenericFactoryOptions
	supportZoneStateCache bool
}

var _ DNSHandlerFactory = &Factory{}

func NewDNSHandlerFactory(typecode string, create DNSHandlerCreatorFunction, disableZoneStateCache ...bool) *Factory {
	disable := false
	for _, b := range disableZoneStateCache {
		disable = disable || b
	}
	return &Factory{
		typecode:              typecode,
		create:                create,
		supportZoneStateCache: !disable,
	}
}

func (this *Factory) SetOptionSourceCreator(creator extension.OptionSourceCreator, defaults ...GenericFactoryOptions) *Factory {
	this.optionCreator = creator
	return this.SetGenericFactoryOptionDefaults(defaults...)
}

func (this *Factory) SetGenericFactoryOptionDefaults(defaults ...GenericFactoryOptions) *Factory {
	if len(defaults) == 1 {
		this.genericDefaults = &defaults[0]
	}
	if len(defaults) > 1 {
		panic("invalid call to SetGenericFactoryOptionDefaults: only one default possible")
	}
	return this
}

func (this *Factory) SetOptionSourceByExample(proto config.OptionSource, defaults ...GenericFactoryOptions) *Factory {
	this.optionCreator = controller.OptionSourceCreator(proto)
	return this.SetGenericFactoryOptionDefaults(defaults...)
}

////////////////////////////////////////////////////////////////////////////////

func (this *Factory) IsResponsibleFor(object *dnsutils.DNSProviderObject) bool {
	return object.DNSProvider().Spec.Type == this.typecode
}

func (this *Factory) TypeCodes() utils.StringSet {
	return utils.NewStringSet(this.typecode)
}

func (this *Factory) Name() string {
	return this.typecode
}

func (this *Factory) Create(typecode string, config *DNSHandlerConfig) (DNSHandler, error) {
	if typecode == this.typecode {
		return this.create(config)
	}
	return nil, fmt.Errorf("not responsible for %q", typecode)
}

func (this *Factory) CreateOptionSource() (config.OptionSource, *GenericFactoryOptions) {
	if this.optionCreator != nil {
		return this.optionCreator(), this.genericDefaults
	}
	return nil, this.genericDefaults
}

func (this *Factory) SupportZoneStateCache(typecode string) (bool, error) {
	if typecode == this.typecode {
		return this.supportZoneStateCache, nil
	}
	return false, fmt.Errorf("not responsible for %q", typecode)
}

///////////////////////////////////////////////////////////////////////////////

type CompoundFactory struct {
	name       string
	typecodes  utils.StringSet
	finalizers utils.StringSet
	factories  map[string]DNSHandlerFactory
}

var _ DNSHandlerFactory = &CompoundFactory{}

func NewDNSHandlerCompoundFactory(name string) *CompoundFactory {
	return &CompoundFactory{name,
		utils.StringSet{}, utils.StringSet{},
		map[string]DNSHandlerFactory{}}
}

func (this *CompoundFactory) Add(f DNSHandlerFactory, finalizer ...string) error {
	typecodes := f.TypeCodes()
	for t := range typecodes {
		if this.typecodes.Contains(t) {
			return fmt.Errorf("typecode %q already registered at compund factory %s", t, this.name)
		}
	}
	for t := range typecodes {
		this.factories[t] = f
		this.typecodes.Add(t)
		this.finalizers.Add(controller.FinalizerName("dns.gardener.cloud", t))
	}
	this.finalizers.Add(finalizer...)
	return nil
}

func (this *CompoundFactory) IsResponsibleFor(object *dnsutils.DNSProviderObject) bool {
	_, ok := this.factories[object.DNSProvider().Spec.Type]
	return ok
}

func (this *CompoundFactory) TypeCodes() utils.StringSet {
	return this.typecodes.Copy()
}

func (this *CompoundFactory) Name() string {
	return this.name
}

func (this *CompoundFactory) Finalizers() utils.StringSet {
	return this.finalizers.Copy()
}

func (this *CompoundFactory) Create(typecode string, cfg *DNSHandlerConfig) (DNSHandler, error) {
	f := this.factories[typecode]
	if f != nil {
		if cfg.Options != nil {
			compound := cfg.Options.Options.(config.OptionSet)
			local := *cfg
			cfg = &local
			src := compound.GetSource(f.Name())
			if src != nil {
				local.Options = GetFactoryOptions(src)
			} else {
				local.Options = nil
			}
		}
		return f.Create(typecode, cfg)
	}
	return nil, fmt.Errorf("not responsible for %q", typecode)
}

func HandlerStringMapper(name string) func(s string) string {
	return func(s string) string {
		return fmt.Sprintf("%s for provider type %q", s, name)
	}
}

func (this *CompoundFactory) CreateOptionSource() (config.OptionSource, *GenericFactoryOptions) {
	found := false
	compound := config.NewSharedOptionSet("compound", "")
	for n, f := range this.factories {
		src := CreateFactoryOptionSource(f, n)
		if src != nil {
			compound.AddSource(n, src)
			found = true
		}
	}
	if found {
		return compound, nil
	}
	return nil, nil
}

func (this *CompoundFactory) SupportZoneStateCache(typecode string) (bool, error) {
	f := this.factories[typecode]
	if f != nil {
		return f.SupportZoneStateCache(typecode)
	}
	return false, fmt.Errorf("not responsible for %q", typecode)
}
