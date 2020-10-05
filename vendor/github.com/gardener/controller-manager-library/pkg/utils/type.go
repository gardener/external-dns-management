/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package utils

import (
	"fmt"
	"reflect"
)

func TypeKey(v interface{}) (reflect.Type, error) {
	t, ok := v.(reflect.Type)
	if !ok {
		t = reflect.TypeOf(v)
	}
	if t == nil {
		return nil, fmt.Errorf("invalid type spec %s(%T)", v, v)
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t, nil
}
