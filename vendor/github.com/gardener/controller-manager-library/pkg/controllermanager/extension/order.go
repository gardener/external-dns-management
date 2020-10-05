/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package extension

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

type OrderedElem interface {
	Name() string
	Before() []string
	After() []string
}

var orderedelem *OrderedElem
var t_orderedelem reflect.Type

func init() {
	t_orderedelem = reflect.TypeOf(orderedelem).Elem()
}

func ordered(a interface{}) map[string]OrderedElem {
	if utils.IsNil(a) {
		return nil
	}
	av := reflect.ValueOf(a)
	at := av.Type()
	result := map[string]OrderedElem{}

	switch av.Kind() {
	case reflect.Map:
		if at.Key().Kind() != reflect.String {
			panic(fmt.Sprintf("%T is no string map", a))
		}
		if at.Elem() == t_orderedelem {
			return a.(map[string]OrderedElem)
		}
		if !at.Elem().ConvertibleTo(t_orderedelem) {
			panic(fmt.Sprintf("%s/%s is no OrderedElem", at.Elem().PkgPath(), at.Elem()))
		}
		it := av.MapRange()
		for it.Next() {
			result[it.Key().String()] = it.Value().Interface().(OrderedElem)
		}
	case reflect.Slice, reflect.Array:
		if at.Elem() == t_orderedelem {
			for _, e := range a.([]OrderedElem) {
				result[e.Name()] = e
			}
		} else {
			if !at.Elem().ConvertibleTo(t_orderedelem) {
				panic(fmt.Sprintf("%s/%s is no OrderedElem", at.Elem().PkgPath(), at.Elem()))
			}
			for i := 0; i < av.Len(); i++ {
				e := av.Index(i).Interface().(OrderedElem)
				result[e.Name()] = e
			}
		}
		return result
	default:
		panic(fmt.Sprintf("%T is no map or array", a))
	}
	return result
}

func Order(m interface{}) ([]string, map[string][]string, error) {
	base := ordered(m)
	after := map[string][]string{}

	for n, d := range base {
		for _, a := range d.After() {
			if base[a] != nil {
				after[n] = append(after[n], a)
			}
		}
		for _, b := range d.Before() {
			if base[b] != nil {
				after[b] = append(after[b], n)
			}
		}
	}
	order := []string{}

	for n := range base {
		err := _order(base, []string{}, &order, n, after)
		if err != nil {
			return nil, after, err
		}
	}
	return order, after, nil
}

func cycle(a []string) []string {
	if len(a) == 0 {
		return a
	}
	min := 0
	for i, n := range a {
		if strings.Compare(a[min], n) > 0 {
			min = i
		}
	}
	return append(a[min:], append(a[:min], a[min])...)
}

func _order(elems map[string]OrderedElem, history []string, order *[]string, name string, after map[string][]string) error {
	if elems[name] == nil {
		return nil
	}
	for i, n := range history {
		if n == name {
			return fmt.Errorf("cycle detected: %+v", cycle(history[i:]))
		}
	}
	for _, n := range *order {
		if n == name {
			return nil
		}
	}
	history = append(history, name)
	for _, a := range after[name] {
		err := _order(elems, history, order, a, after)
		if err != nil {
			return err
		}
	}
	*order = append(*order, name)
	return nil
}
