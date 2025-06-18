// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import "k8s.io/apimachinery/pkg/util/sets"

// Properties is a map of string keys to string values.
type Properties map[string]string

// Has returns true if the property with the given key exists.
func (p Properties) Has(k string) bool {
	_, ok := p[k]
	return ok
}

// Equals returns true if the Properties map is equal to the given map.
func (p Properties) Equals(t map[string]string) bool {
	for k, v := range p {
		tv, ok := t[k]
		if !ok || tv != v {
			return false
		}
	}
	for k := range t {
		if !p.Has(k) {
			return false
		}
	}
	return true
}

// Clone returns a copy of the Properties map.
func (p Properties) Clone() Properties {
	new := Properties{}
	for k, v := range p {
		new[k] = v
	}
	return new
}

// Keys returns a set of all keys in the Properties map.
func (p Properties) Keys() sets.Set[string] {
	new := sets.New[string]()
	for k := range p {
		new.Insert(k)
	}
	return new
}
