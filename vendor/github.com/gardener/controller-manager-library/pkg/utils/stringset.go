/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package utils

import (
	"fmt"
	"reflect"
	"strings"
)

type StringSet map[string]struct{}

func NewStringSet(a ...string) StringSet {
	return StringSet{}.Add(a...)
}

func NewStringSetByArray(a []string) StringSet {
	s := StringSet{}
	if a != nil {
		s.Add(a...)
	}
	return s
}

func NewStringSetBySets(sets ...StringSet) StringSet {
	s := StringSet{}
	for _, set := range sets {
		for a := range set {
			s.Add(a)
		}
	}
	return s
}

func (this StringSet) String() string {
	sep := ""
	data := "["
	for k := range this {
		data = fmt.Sprintf("%s%s'%s'", data, sep, k)
		sep = ", "
	}
	return data + "]"
}

func (this StringSet) Clear() {
	for n := range this {
		delete(this, n)
	}
}

func (this StringSet) IsEmpty() bool {
	return len(this) == 0
}

func (this StringSet) Contains(n string) bool {
	if this == nil {
		return false
	}
	_, ok := this[n]
	return ok
}

func (this StringSet) Remove(n ...string) StringSet {
	for _, p := range n {
		delete(this, p)
	}
	return this
}

func (this StringSet) AddAll(n []string) StringSet {
	return this.Add(n...)
}

func (this StringSet) Add(n ...string) StringSet {
	for _, p := range n {
		this[p] = struct{}{}
	}
	return this
}

func (this StringSet) AddSet(sets ...StringSet) StringSet {
	for _, s := range sets {
		for e := range s {
			this.Add(e)
		}
	}
	return this
}

func (this StringSet) RemoveSet(sets ...StringSet) StringSet {
	for _, s := range sets {
		for e := range s {
			this.Remove(e)
		}
	}
	return this
}

func (this StringSet) AddAllSplitted(n string, seps ...string) StringSet {
	return this.AddAllSplittedSelected(n, StandardStringElement, seps...)
}

func StandardStringElement(s string) (string, bool) {
	return strings.ToLower(strings.TrimSpace(s)), true
}

func StandardNonEmptyStringElement(s string) (string, bool) {
	s, _ = StandardStringElement(s)
	return s, s != ""
}

func NonEmptyStringElement(s string) (string, bool) {
	s = strings.TrimSpace(s)
	return s, s != ""
}

func StringElement(s string) (string, bool) {
	return strings.TrimSpace(s), true
}

func (this StringSet) AddAllSplittedSelected(n string, sel func(s string) (string, bool), seps ...string) StringSet {
	sep := ","
	if len(seps) > 0 {
		sep = seps[0]
	}
	for _, p := range strings.Split(n, sep) {
		if v, ok := sel(p); ok {
			this.Add(v)
		}
	}
	return this
}

func (this StringSet) Equals(set StringSet) bool {
	for n := range set {
		if !this.Contains(n) {
			return false
		}
	}
	for n := range this {
		if !set.Contains(n) {
			return false
		}
	}
	return true
}

func (this StringSet) DiffFrom(set StringSet) (add, del StringSet) {
	add = StringSet{}
	del = StringSet{}
	for n := range set {
		if !this.Contains(n) {
			add.Add(n)
		}
	}
	for n := range this {
		if !set.Contains(n) {
			del.Add(n)
		}
	}
	return
}

func (this StringSet) Copy() StringSet {
	set := NewStringSet()
	for n := range this {
		set[n] = struct{}{}
	}
	return set
}

func (this StringSet) Intersect(o StringSet) StringSet {
	set := NewStringSet()
	for n := range this {
		if o.Contains(n) {
			set[n] = struct{}{}
		}
	}
	return set
}

func (this StringSet) AsArray() []string {
	a := []string{}
	for n := range this {
		a = append(a, n)
	}
	return a
}

func StringKeySet(anystringkeymap interface{}) StringSet {
	ret := StringSet{}
	if anystringkeymap == nil {
		return ret
	}
	v := reflect.ValueOf(anystringkeymap)

	for _, keyValue := range v.MapKeys() {
		ret.Add(keyValue.Interface().(string))
	}
	return ret
}

func StringKeyArray(anystringkeymap interface{}) []string {
	if anystringkeymap == nil {
		return nil
	}
	v := reflect.ValueOf(anystringkeymap)
	keys := v.MapKeys()
	ret := make([]string, len(keys))

	for i, keyValue := range keys {
		ret[i] = keyValue.Interface().(string)
	}
	return ret
}

func StringValueSet(anystringvaluemap interface{}) StringSet {
	v := reflect.ValueOf(anystringvaluemap)
	ret := StringSet{}

	for i := v.MapRange(); i.Next(); {
		ret.Add(i.Value().Interface().(string))
	}
	return ret
}
