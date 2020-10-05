/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package config

import (
	"fmt"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type OptionValidator func(name string, set OptionSet)

type Validatable interface {
	SetValidator(validator OptionValidator)
}

////////////////////////////////////////////////////////////////////////////////

type DefaultOptionSet struct {
	OptionSetProxy
	lock      sync.Mutex
	name      string
	nested    sync.Mutex
	prefix    string
	completed bool
	validator OptionValidator

	nestedSources map[string]OptionSource

	renamedFlags     map[string]*pflag.Flag
	arbitraryOptions map[string]*ArbitraryOption
	flags            *pflag.FlagSet
}

var _ OptionSet = &DefaultOptionSet{}

func NewDefaultOptionSet(name, prefix string) *DefaultOptionSet {
	s := &DefaultOptionSet{
		name:             name,
		prefix:           prefix,
		renamedFlags:     map[string]*pflag.Flag{},
		arbitraryOptions: map[string]*ArbitraryOption{},
		nestedSources:    map[string]OptionSource{},
		flags:            pflag.NewFlagSet(prefix, pflag.ExitOnError),
	}
	s.OptionSetProxy = OptionSetProxy(s.AddOption)
	return s
}

func (this *DefaultOptionSet) SetValidator(v OptionValidator) {
	this.validator = v
}

func (this *DefaultOptionSet) Name() string {
	if this.name != "" {
		return this.name
	}
	return this.prefix
}

func (this *DefaultOptionSet) Prefix() string {
	return this.prefix
}

func (this *DefaultOptionSet) GetOption(name string) *ArbitraryOption {
	return this.arbitraryOptions[name]
}

func (this *DefaultOptionSet) GetSource(key string) OptionSource {
	return this.nestedSources[key]
}

func (this *DefaultOptionSet) VisitOptions(f OptionVisitor) {
	for _, o := range this.arbitraryOptions {
		if !f(o) {
			return
		}
	}
}

func (this *DefaultOptionSet) VisitSources(f OptionSourceVisitor) {
	for n, t := range this.nestedSources {
		if !f(n, t) {
			return
		}
	}
}

func (this *DefaultOptionSet) checkMod() {
	if this.completed {
		panic("option set already completed")
	}
}

func (this *DefaultOptionSet) AddSource(key string, src OptionSource) {
	if src == nil {
		return
	}
	this.checkMod()
	this.nested.Lock()
	defer this.nested.Unlock()

	if _, ok := this.nestedSources[key]; ok {
		panic(fmt.Sprintf("source key %q already in use", key))
	}
	for k, t := range this.nestedSources {
		if t == src {
			panic(fmt.Sprintf("source %q already registered with key %q", key, k))
		}
	}
	this.nestedSources[key] = src
}

func (this *DefaultOptionSet) addOption(flag, renamed *pflag.Flag, otype OptionType, target interface{}, name string, def interface{}, desc string) interface{} {
	if flag != nil {
		this.flags.AddFlag(flag)
	}
	if renamed != nil {
		this.renamedFlags[name] = renamed
	}
	n := &ArbitraryOption{
		Name:        name,
		Type:        otype,
		Target:      target,
		Default:     def,
		Description: desc,
		FlagSet:     this.flags,
	}
	this.arbitraryOptions[name] = n
	return n.Target
}

func (this *DefaultOptionSet) AddRenamedOption(opt *ArbitraryOption, name, short string, desc string) interface{} {
	this.checkMod()
	this.lock.Lock()
	defer this.lock.Unlock()

	var renamed *pflag.Flag

	target := opt.Target
	flag := opt.Flag()

	if name != opt.Name {
		renamed = flag
		copy := *flag
		copy.Name = name
		copy.Shorthand = short
		if desc != "" {
			copy.Usage = desc
		}
		flag = &copy
	}
	return this.addOption(flag, renamed, opt.Type, target, name, opt.Default, flag.Usage)
}

func (this *DefaultOptionSet) AddOption(otype OptionType, target interface{}, name, short string, def interface{}, desc string) interface{} {
	this.checkMod()
	this.lock.Lock()
	defer this.lock.Unlock()

	old := this.arbitraryOptions[name]
	if old != nil {
		panic(fmt.Sprintf("option %q already defined for %q", name, this.Name()))
	}
	if this.validator != nil {
		this.validator(name, this)
	}
	target = otype.AddToFlags(this.flags, target, name, short, def, desc)
	return this.addOption(nil, nil, otype, target, name, def, desc)
}

func (this *DefaultOptionSet) AddOptionsToSet(set OptionSet) {
	this.Complete()
	this.lock.Lock()
	defer this.lock.Unlock()

	for _, o := range this.arbitraryOptions {
		this.addOptionToSet(o, set, o.Description)
	}
}

func (this *DefaultOptionSet) addOptionToSet(o *ArbitraryOption, set OptionSet, desc string) {
	flag := o.FlagSet.Lookup(o.Name)
	name := o.Name
	short := flag.Shorthand
	if this.prefix != "" {
		name = this.prefix + "." + name
		short = ""
	}
	set.AddRenamedOption(o, name, short, desc)
}

func (this *DefaultOptionSet) AddToFlags(flags *pflag.FlagSet) {
	this.Complete()
	this.lock.Lock()
	defer this.lock.Unlock()

	for _, o := range this.arbitraryOptions {
		flag := o.FlagSet.Lookup(o.Name)
		name := o.Name
		if this.prefix != "" {
			name = this.prefix + "." + name
			o.Type.AddToFlags(flags, o.Target, name, "", o.Default, flag.Usage)
		} else {
			flags.AddFlag(flag)
		}
	}
}

func (this *DefaultOptionSet) Complete() {
	this.nested.Lock()
	defer this.nested.Unlock()
	this.complete()
}

func (this *DefaultOptionSet) complete() {
	if !this.completed {
		for _, nested := range this.nestedSources {
			// fmt.Printf("adding nested %q <- %q\n", this.name, n)
			nested.AddOptionsToSet(this)
		}
		// fmt.Printf("%q completed\n", this.name)
		this.completed = true
	}
}

func (this *DefaultOptionSet) Evaluate() error {
	this.evalRenamed()
	return this.evalNested()
}

func (this *DefaultOptionSet) evalRenamed() {
	this.lock.Lock()
	defer this.lock.Unlock()
	for name, renamed := range this.renamedFlags {
		flag := this.flags.Lookup(name)
		renamed.Changed = flag.Changed
	}
}

func (this *DefaultOptionSet) evalNested() error {
	this.nested.Lock()
	defer this.nested.Unlock()

	for _, nested := range this.nestedSources {
		// fmt.Printf("try eval nested\n")
		if e, ok := nested.(OptionEvaluator); ok {
			// fmt.Printf("eval nested\n")
			err := e.Evaluate()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (this *DefaultOptionSet) AddToCommand(cmd *cobra.Command) {
	this.AddToFlags(cmd.PersistentFlags())
}
