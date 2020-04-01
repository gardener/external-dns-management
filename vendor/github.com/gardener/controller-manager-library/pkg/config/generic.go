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

package config

import (
	"fmt"
	"sync"
	"time"
)

///////////////////////////////////////////////////////////////////////////////
// Option Kinds

type OptionKind func(t *GenericOptionSource) OptionSet

// Flat options are just added as they are to an outer option sink
func Flat(t *GenericOptionSource) OptionSet {
	return t.Flat()
}

// Prefixed options are just added with a name prefix to an outer option sink
func Prefixed(t *GenericOptionSource) OptionSet {
	return t.Prefixed()
}

// Shared options are flat options that can be shared with other OptionSources
// (if they are also shared). The value of this option is distributed to
// all shared option targets.
func Shared(t *GenericOptionSource) OptionSet {
	return t.Shared()
}

// PrefixedShared options are prefixed options that additionally declares a
// shared flat option on the sink. If the flat option is changed the value is propagated
// to option sources declaring this shared option. If the perfixed option is set,
// only this source get the value.
func PrefixedShared(t *GenericOptionSource) OptionSet {
	return t.PrefixedShared()
}

///////////////////////////////////////////////////////////////////////////////
// Generic OptionSource for arbitrary options of different kinds by offering
// OptionSets for all kind of options. By itself it is not an OptionSet but
// offers methods for adding options of dedicated kinds.

type GenericOptionSource struct {
	lock         sync.Mutex
	completelock sync.Mutex

	completed  bool
	name       string
	prefix     string
	descMapper StringMapper

	options map[string]OptionSet

	flat           OptionSet
	prefixed       OptionSet
	shared         OptionSet
	prefixedShared OptionSet
}

var _ Options = (*GenericOptionSource)(nil)
var _ OptionSource = (*GenericOptionSource)(nil)
var _ OptionSourceSource = (*GenericOptionSource)(nil)
var _ OptionGroup = (*GenericOptionSource)(nil)

func NewGenericOptionSource(name, prefix string, descmapper StringMapper) *GenericOptionSource {
	return &GenericOptionSource{
		name:       name,
		prefix:     prefix,
		descMapper: descmapper,
		options:    map[string]OptionSet{},
	}
}

func (this *GenericOptionSource) Name() string {
	if this.name != "" {
		return this.name
	}
	return this.prefix
}

func (this *GenericOptionSource) Prefix() string {
	return this.prefix
}

func (this *GenericOptionSource) GetOption(name string) *ArbitraryOption {
	if set, ok := this.options[name]; ok {
		return set.GetOption(name)
	}
	return nil
}

func (this *GenericOptionSource) VisitOptions(f OptionVisitor) {
	for n, o := range this.options {
		if !f(o.GetOption(n)) {
			return
		}
	}
}

func (this *GenericOptionSource) GetSource(name string) OptionSource {
	switch name {
	case this.name + ".flat":
		return this.flat
	case this.name + ".shared":
		return this.shared
	case this.name + ".prefixed":
		return this.flat
	case this.name + ".prefixedshared":
		return this.prefixedShared
	}
	return nil
}

func (this *GenericOptionSource) addOption(name string, set OptionSet) {
	if s, ok := this.options[name]; ok {
		panic(fmt.Sprintf("option %q already declared for targets %q (%s)", name, this.name, s.Name()))
	}
	this.options[name] = set
}

func (this *GenericOptionSource) getSet(t *OptionSet, f func() OptionSet) OptionSet {
	this.lock.Lock()
	defer this.lock.Unlock()

	if *t == nil {
		if this.completed {
			return nil
		}
		*t = f()
		(*t).(Validatable).SetValidator(this.addOption)
	}
	return *t
}

func (this *GenericOptionSource) Flat() OptionSet {
	return this.getSet(&this.flat, func() OptionSet {
		return NewDefaultOptionSet(this.name+".flat", "")
	})
}

func (this *GenericOptionSource) Prefixed() OptionSet {
	return this.getSet(&this.prefixed, func() OptionSet {
		return NewDefaultOptionSet(this.name+".prefixed", this.prefix)
	})
}

func (this *GenericOptionSource) Shared() OptionSet {
	return this.getSet(&this.shared, func() OptionSet {
		return NewSharedOptionSet(this.name+".shared", "", nil)
	})
}

func (this *GenericOptionSource) PrefixedShared() OptionSet {
	return this.getSet(&this.prefixedShared, func() OptionSet {
		return NewSharedOptionSet(this.name+".prefixedshared", this.prefix, this.descMapper)
	})
}

func (this *GenericOptionSource) VisitSources(f OptionSourceVisitor) {
	this.call(func(set OptionSet) interface{} {
		if !f(set.Name(), set) {
			return false
		}
		return nil
	})
}

func (this *GenericOptionSource) call(f func(OptionSet) interface{}) interface{} {
	if this.flat != nil {
		if r := f(this.flat); r != nil {
			return r
		}
	}
	if this.prefixed != nil {
		if r := f(this.prefixed); r != nil {
			return r
		}
	}
	if this.shared != nil {
		if r := f(this.shared); r != nil {
			return r
		}
	}
	if this.prefixedShared != nil {
		if r := f(this.prefixedShared); r != nil {
			return r
		}
	}
	return nil
}

func (this *GenericOptionSource) checkMod() {
	this.completelock.Lock()
	defer this.completelock.Unlock()
	if this.completed {
		panic("option set already completed")
	}
}

func (this *GenericOptionSource) AddOptionsToSet(set OptionSet) {
	this.call(func(targets OptionSet) interface{} { targets.AddOptionsToSet(set); return nil })
}

func (this *GenericOptionSource) Complete() {
	this.completelock.Lock()
	defer this.completelock.Unlock()

	if !this.completed {
		this.call(func(targets OptionSet) interface{} { targets.Complete(); return nil })
		this.completed = true
	}
}

func (this *GenericOptionSource) Evaluate() error {
	err := this.call(func(targets OptionSet) interface{} { return targets.Evaluate() })
	if err != nil {
		return err.(error)
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////

func (this *GenericOptionSource) AddOption(kind OptionKind, otype OptionType, target interface{}, name, short string, def interface{}, desc string) interface{} {
	return kind(this).AddOption(otype, target, name, short, def, desc)
}

func (this *GenericOptionSource) AddStringOption(kind OptionKind, target *string, name, short string, def string, desc string) *string {
	return this.AddOption(kind, StringOption, target, name, short, def, desc).(*string)
}
func (this *GenericOptionSource) AddStringArrayOption(kind OptionKind, target *[]string, name, short string, def []string, desc string) *[]string {
	return this.AddOption(kind, StringArrayOption, target, name, short, def, desc).(*[]string)
}
func (this *GenericOptionSource) AddIntOption(kind OptionKind, target *int, name, short string, def int, desc string) *int {
	return this.AddOption(kind, IntOption, target, name, short, def, desc).(*int)
}
func (this *GenericOptionSource) AddUintOption(kind OptionKind, target *uint, name, short string, def uint, desc string) *uint {
	return this.AddOption(kind, UintOption, target, name, short, def, desc).(*uint)
}
func (this *GenericOptionSource) AddBoolOption(kind OptionKind, target *bool, name, short string, def bool, desc string) *bool {
	return this.AddOption(kind, BoolOption, target, name, short, def, desc).(*bool)
}
func (this *GenericOptionSource) AddDurationOption(kind OptionKind, target *time.Duration, name, short string, def time.Duration, desc string) *time.Duration {
	return this.AddOption(kind, DurationOption, target, name, short, def, desc).(*time.Duration)
}
