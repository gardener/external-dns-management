/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package kutil

import (
	"reflect"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func IsListType(t reflect.Type) (reflect.Type, bool) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	field, ok := t.FieldByName("Items")
	if !ok {
		return nil, false
	}

	t = field.Type
	if t.Kind() != reflect.Slice {
		return nil, false
	}
	t = t.Elem()
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, false
	}
	return t, true
}

var unstructuredType = reflect.TypeOf(unstructured.Unstructured{})
var unstructuredListType = reflect.TypeOf(unstructured.UnstructuredList{})

func DetermineListType(s *runtime.Scheme, gv schema.GroupVersion, t reflect.Type) reflect.Type {
	if t == unstructuredType {
		return unstructuredListType
	}
	for _gvk, _t := range s.AllKnownTypes() {
		if gv == _gvk.GroupVersion() {
			e, ok := IsListType(_t)
			if ok && e == t {
				return _t
			}
		}
	}
	return nil
}
