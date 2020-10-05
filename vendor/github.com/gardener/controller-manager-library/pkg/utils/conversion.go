/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package utils

import (
	"fmt"
	"reflect"
)

var v_interface interface{}
var t_interface = reflect.TypeOf(v_interface)

func ToInterfaceSlice(a interface{}) []interface{} {
	if a == nil {
		return nil
	}
	v := reflect.ValueOf(a)

	switch v.Kind() {
	case reflect.Array:
		r := make([]interface{}, v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			r[i] = v.Index(i).Interface()
		}
		return r
	case reflect.Slice:
		if v.Type().Elem() == t_interface {
			return a.([]interface{})
		}
		r := make([]interface{}, v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			r[i] = v.Index(i).Interface()
		}
		return r
	default:
		panic(fmt.Sprintf("invalid argument type: %s", v.Type()))
	}
}
