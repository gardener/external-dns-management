/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"fmt"
	"sync"
)

// Composition handles the mapping of cluster names for a dedicate controller
// _Definition in dedicated composition of controllers in a controller
// manager
type Composition interface {
	Map(name string) string
	MapInfo(name string) string
}

type _Composition struct {
	mapping map[string]string
}

var _ Composition = &_Composition{}

func (this *_Composition) Map(name string) string {
	t := this.mapping[name]
	if t != "" {
		return t
	}
	return name
}

func (this *_Composition) MapInfo(name string) string {
	t := this.mapping[name]
	if t != "" {
		return fmt.Sprintf("%q (mapped to %q)", name, t)
	}
	return name
}

var lock sync.Mutex
var compositions = map[string]Composition{}
var identity = &_Composition{map[string]string{}}

type mapping struct {
	composition *_Composition
}

func GetComposition(name string) Composition {
	lock.Lock()
	defer lock.Unlock()
	c := compositions[name]
	if c == nil {
		return identity
	}
	return c
}

////////////////////////////////////////////////////////////////////////////////
// configuration

func Map() *mapping {
	return &mapping{&_Composition{map[string]string{}}}
}

func (this *mapping) copy() *mapping {
	m := Map()
	for s, t := range this.composition.mapping {
		m.composition.mapping[s] = t
	}
	return m
}

func (this *mapping) Cluster(name, to string) *mapping {
	m := this.copy()
	m.composition.mapping[name] = to
	return m
}

func (this *mapping) MustRegister(name string) *mapping {
	err := this.Register(name)
	if err != nil {
		panic(err)
	}
	return this
}

func (this *mapping) Register(name string) error {
	lock.Lock()
	defer lock.Unlock()
	if compositions[name] != nil {
		return fmt.Errorf("duplication composition info for controller %q", name)
	}
	compositions[name] = this.composition
	return nil
}
