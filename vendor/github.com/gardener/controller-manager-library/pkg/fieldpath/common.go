/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package fieldpath

import (
	"reflect"
)

type Path interface {
	Get(obj interface{}) (interface{}, error)
	Set(obj interface{}, value interface{}) error
}

func Values(n Node, v interface{}) ([]interface{}, error) {
	r, err := n.Get(v)
	if r == nil || err != nil {
		return nil, err
	}
	rv := reflect.ValueOf(r)
	if rv.Kind() != reflect.Slice {
		return []interface{}{r}, nil
	}

	lvl := 0
	for n != nil {
		if p, ok := n.(*ProjectionNode); ok {
			lvl++
			n = p.path
		} else {
			n = nil
		}
	}

	if lvl == 0 {
		return []interface{}{r}, nil
	}

	return flatten(rv, lvl, nil), nil
}

func flatten(rv reflect.Value, lvl int, f []interface{}) []interface{} {
	for i := 0; i < rv.Len(); i++ {
		if lvl == 1 {
			f = append(f, rv.Index(i).Interface())
		} else {
			f = flatten(rv.Index(i), lvl-1, f)
		}
	}
	return f
}
