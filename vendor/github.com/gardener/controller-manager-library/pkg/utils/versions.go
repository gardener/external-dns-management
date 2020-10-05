/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package utils

import (
	"fmt"
	"reflect"

	"github.com/Masterminds/semver"
)

// Versioned maintains a set of provided object versions and yields the best
// such version for given required version.
type Versioned struct {
	etype    reflect.Type
	versions map[string]interface{}
	def      interface{}
}

func NewVersioned(proto interface{}) *Versioned {
	t, ok := proto.(reflect.Type)
	if !ok {
		t = reflect.TypeOf(proto)
	}
	return &Versioned{t, map[string]interface{}{}, nil}
}

func (this *Versioned) SetDefault(obj interface{}) error {
	if reflect.TypeOf(obj) != this.etype {
		return fmt.Errorf("invalid element type, found %s, but expected %s", reflect.TypeOf(obj), this.etype)
	}
	this.def = obj
	return nil
}

func (this *Versioned) GetDefault() interface{} {
	return this.def
}

func (this *Versioned) RegisterVersion(v *semver.Version, obj interface{}) error {
	if reflect.TypeOf(obj) != this.etype {
		return fmt.Errorf("invalid element type, found %s, but expected %s", reflect.TypeOf(obj), this.etype)
	}
	this.versions[v.String()] = obj
	return nil
}

func (this *Versioned) MustRegisterVersion(v *semver.Version, obj interface{}) {
	err := this.RegisterVersion(v, obj)
	if err != nil {
		panic(fmt.Sprintf("cannot register versioned object: %s", err))
	}
}

func (this *Versioned) GetFor(req *semver.Version) interface{} {
	var found *semver.Version
	obj := this.def

	for v, o := range this.versions {
		vers, _ := semver.NewVersion(v)
		if !vers.GreaterThan(req) {
			if found == nil || vers.GreaterThan(found) {
				found, obj = vers, o
			}
		}
	}
	return obj
}

func (this *Versioned) GetVersions() map[*semver.Version]interface{} {
	result := map[*semver.Version]interface{}{}
	for v, o := range this.versions {
		vers, _ := semver.NewVersion(v)
		result[vers] = o
	}
	if this.def != nil {
		result[nil] = this.def
	}
	return result
}
