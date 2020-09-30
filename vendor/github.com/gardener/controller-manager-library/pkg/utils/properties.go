/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package utils

type Properties map[string]string

func (this Properties) Has(k string) bool {
	_, ok := this[k]
	return ok
}

func (this Properties) Equals(t map[string]string) bool {
	for k, v := range this {
		tv, ok := t[k]
		if !ok || tv != v {
			return false
		}
	}
	for k := range t {
		if !this.Has(k) {
			return false
		}
	}
	return true
}

func (this Properties) Copy() Properties {
	new := Properties{}
	for k, v := range this {
		new[k] = v
	}
	return new
}

func (this Properties) Keys() StringSet {
	new := StringSet{}
	for k := range this {
		new.Add(k)
	}
	return new
}
