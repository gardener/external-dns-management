/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fieldpath

import (
	"fmt"
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

type ValueGetter interface {
	Get(obj interface{}) (interface{}, error)
}

type Field interface {
	BaseType() reflect.Type
	Type() reflect.Type
	Get(base interface{}) (interface{}, error)
	GetAsValue(base interface{}) (interface{}, error)
	Set(base interface{}, value interface{}) error

	String() string
}

type field struct {
	node      Node
	baseType  reflect.Type
	fieldType reflect.Type
}

var _ Field = &field{}

func NewField(base interface{}, path string) (Field, error) {
	n, err := Compile(path)
	if err != nil {
		return nil, err
	}
	t, err := n.Type(base)
	if err != nil {
		return nil, err
	}
	return &field{n, valueType(reflect.TypeOf(base)), t}, nil
}

func RequiredField(base interface{}, path string) Field {
	f, err := NewField(base, path)
	if err != nil {
		panic(fmt.Sprintf("FieldNode %q for %T is invalid: %s", path, base, err))
	}
	return f
}

func (this *field) String() string {
	return fmt.Sprintf("%s%s", this.baseType, this.node)
}

func (this *field) Type() reflect.Type {
	return this.fieldType
}
func (this *field) BaseType() reflect.Type {
	return this.baseType
}
func (this *field) Get(base interface{}) (interface{}, error) {
	if valueType(reflect.TypeOf(base)) != this.baseType {
		return nil, fmt.Errorf("invalid base element: got %T, expected %s", base, this.baseType)
	}
	v, err := this.node.Get(base)
	if utils.IsNil(v) {
		return nil, err
	}
	return v, err
}

func (this *field) GetAsValue(base interface{}) (interface{}, error) {
	v, err := this.Get(base)
	if err != nil {
		return nil, err
	}
	if utils.IsNil(v) {
		return nil, nil
	}
	value := reflect.ValueOf(v)
	if value.IsNil() {
		return nil, nil
	}
	for value.Kind() == reflect.Ptr {
		value = value.Elem()
	}
	return value.Interface(), nil
}

func (this *field) Set(base interface{}, value interface{}) error {
	if reflect.TypeOf(base) != reflect.PtrTo(this.baseType) {
		return fmt.Errorf("invalid base element: got %T, expected %s", base, reflect.PtrTo(this.baseType))
	}
	return this.node.Set(base, value)
}
