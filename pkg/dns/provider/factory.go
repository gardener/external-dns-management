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
	"github.com/gardener/controller-manager-library/pkg/utils"

	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type DNSHandlerCreatorFunction func(config *DNSHandlerConfig) (DNSHandler, error)

type Factory struct {
	typecode              string
	create                DNSHandlerCreatorFunction
	supportZoneStateCache bool
}

var _ DNSHandlerFactory = &Factory{}

func NewDNSHandlerFactory(typecode string, create DNSHandlerCreatorFunction, disableZoneStateCache ...bool) DNSHandlerFactory {
	disable := false
	for _, b := range disableZoneStateCache {
		disable = disable || b
	}
	return &Factory{typecode, create, !disable}
}

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

func (this *Factory) SupportZoneStateCache(typecode string) (bool, error) {
	if typecode == this.typecode {
		return this.supportZoneStateCache, nil
	}
	return false, fmt.Errorf("not responsible for %q", typecode)
}

///////////////////////////////////////////////////////////////////////////////

type CompoundFactory struct {
	name      string
	typecodes utils.StringSet
	factories map[string]DNSHandlerFactory
}

var _ DNSHandlerFactory = &CompoundFactory{}

func NewDNSHandlerCompoundFactory(name string) *CompoundFactory {
	return &CompoundFactory{name, utils.StringSet{}, map[string]DNSHandlerFactory{}}
}

func (this *CompoundFactory) Add(f DNSHandlerFactory) error {
	typecodes := f.TypeCodes()
	for t := range typecodes {
		if this.typecodes.Contains(t) {
			return fmt.Errorf("typecode %q already registered at compund factory %s", t, this.name)
		}
	}
	for t := range typecodes {
		this.factories[t] = f
		this.typecodes.Add(t)
	}
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

func (this *CompoundFactory) Create(typecode string, config *DNSHandlerConfig) (DNSHandler, error) {
	f := this.factories[typecode]
	if f != nil {
		return f.Create(typecode, config)
	}
	return nil, fmt.Errorf("not responsible for %q", typecode)
}

func (this *CompoundFactory) SupportZoneStateCache(typecode string) (bool, error) {
	f := this.factories[typecode]
	if f != nil {
		return f.SupportZoneStateCache(typecode)
	}
	return false, fmt.Errorf("not responsible for %q", typecode)
}
