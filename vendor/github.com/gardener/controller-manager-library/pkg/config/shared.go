/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package config

import (
	"fmt"
	"reflect"
	"strings"
)

type SharedOptionSet struct {
	*DefaultOptionSet
	unshared map[string]bool
	shared   OptionSet

	descriptionMapper StringMapper
}

var _ OptionGroup = (*SharedOptionSet)(nil)

func ChainedStringMapper(mappers ...StringMapper) StringMapper {
	switch len(mappers) {
	case 0:
		return IdenityStringMapper
	case 1:
		return mappers[0]
	default:
		return func(s string) string {
			for _, m := range mappers {
				s = m(s)
			}
			return s
		}
	}
}

func NewSharedOptionSet(name, prefix string, descMappers ...StringMapper) *SharedOptionSet {
	descMapper := ChainedStringMapper(descMappers...)
	s := &SharedOptionSet{
		DefaultOptionSet:  NewDefaultOptionSet(name, prefix),
		unshared:          map[string]bool{},
		descriptionMapper: descMapper,
	}
	return s
}

func (this *SharedOptionSet) Unshare(name string) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.unshared[name] = true
}

func (this *SharedOptionSet) AddOptionsToSet(set OptionSet) {
	this.Complete()
	this.lock.Lock()
	defer this.lock.Unlock()

	this.shared = set
	for name, o := range this.arbitraryOptions {
		unshared := this.unshared[name]
		if this.prefix != "" || unshared {
			this.addOptionToSet(o, set, this.descriptionMapper(o.Description))
		}
		if !unshared {
			if old := set.GetOption(name); old != nil {
				if o.Type != old.Type {
					panic(fmt.Sprintf("type mismatch for shared option %s (%s)", name, this.prefix))
				}
				if strings.Index(old.Description, o.Description) < 0 {
					old.Description += ", " + o.Description
					old.Flag().Usage = o.Description
				}
			} else {
				set.AddOption(o.Type, nil, o.Name, o.Flag().Shorthand, nil, o.Description)
			}
		}
	}
}

func (this *SharedOptionSet) evalShared() {
	this.lock.Lock()
	defer this.lock.Unlock()

	// fmt.Printf("eval shared %s\n", this.prefix)
	for name, o := range this.arbitraryOptions {
		if !this.unshared[name] && !o.Changed() {
			// fmt.Printf("eval shared %s\n", name)
			shared := this.shared.GetOption(name)
			if shared.Changed() {
				value := reflect.ValueOf(shared.Target).Elem()
				// fmt.Printf("   %s changed shared\n", name)
				o.Flag().Changed = true
				reflect.ValueOf(o.Target).Elem().Set(value)
			}
		}
	}
}

func (this *SharedOptionSet) Evaluate() error {
	this.evalShared()
	return this.DefaultOptionSet.Evaluate()
}
