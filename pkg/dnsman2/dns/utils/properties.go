// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import "k8s.io/apimachinery/pkg/util/sets"

type Properties map[string]string

func (p Properties) Has(k string) bool {
	_, ok := p[k]
	return ok
}

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

func (p Properties) Clone() Properties {
	new := Properties{}
	for k, v := range p {
		new[k] = v
	}
	return new
}

func (p Properties) Keys() sets.Set[string] {
	new := sets.New[string]()
	for k := range p {
		new.Insert(k)
	}
	return new
}
