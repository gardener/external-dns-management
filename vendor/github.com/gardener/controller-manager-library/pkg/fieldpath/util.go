/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fieldpath

import (
	"reflect"
	"unicode"
)

var none = reflectValue(reflect.Value{})

func IsIdentifierStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func IsIdentifierPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isSimpleType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Map, reflect.Slice, reflect.Struct, reflect.Array, reflect.Func, reflect.Chan:
		return false
	case reflect.Ptr, reflect.Uintptr, reflect.UnsafePointer:
		return false
	default:
		return true
	}
}

func Value(val interface{}) interface{} {
	if val == nil {
		return nil
	}
	v := reflect.ValueOf(val)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	return v.Interface()
}

////////////////////////////////////////////////////////////////////////////////

func valueType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func isPtr(v value) bool {
	if !v.IsValid() {
		return false
	}
	return v.Kind() == reflect.Ptr
}
