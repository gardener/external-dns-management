/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package fieldpath

import (
	"fmt"
	"reflect"
	"strconv"
)

type Node interface {
	Next() Node
	String() string

	Type(interface{}) (reflect.Type, error)
	Validate(interface{}) error
	ValidateType(interface{}, interface{}) error

	Get(interface{}) (interface{}, error)
	Set(interface{}, interface{}) error

	_value(src value, addMissing bool, prev *node) (value, error)
	value(src value, addMissing bool, prev *node) (value, error)
}

////////////////////////////////////////////////////////////////////////////////

type new interface {
	new(self, next Node) Node
}

type node struct {
	next Node
	self Node
}

func (this *node) new(self, next Node) Node {
	this.self = self
	this.next = next
	return self
}

func (this *node) Next() Node {
	return this.next
}

func (this *node) String() string {
	if this.next == nil {
		return ""
	}
	return this.next.String()
}

func (this *node) Type(src interface{}) (reflect.Type, error) {
	v, ok := src.(reflect.Value)
	if ok {
		return this._type(v)
	}
	t, ok := src.(reflect.Type)
	if ok {
		return this._type(reflect.New(t))
	}
	return this._type(reflect.ValueOf(src))
}

func (this *node) _type(v reflect.Value) (reflect.Type, error) {
	field, err := this._value(reflectValue(v), true, nil)
	if err != nil {
		return nil, err
	}
	return field.Type(), nil
}

func (this *node) Validate(src interface{}) error {
	v, ok := src.(reflect.Value)
	if ok {
		return this._validate(v)
	}
	return this._validate(reflect.ValueOf(src))
}

func (this *node) ValidateType(src interface{}, val interface{}) error {
	v, ok := src.(reflect.Value)
	if !ok {
		v = reflect.ValueOf(src)
	}
	t, ok := val.(reflect.Type)
	if !ok {
		t = reflect.TypeOf(val)
	}
	return this._validateType(v, t)
}

func (this *node) Get(src interface{}) (interface{}, error) {
	v, ok := src.(reflect.Value)
	if ok {
		return this._get(v)
	}
	return this._get(reflect.ValueOf(src))
}

func (this *node) Set(src interface{}, val interface{}) error {
	v, ok := src.(reflect.Value)
	if ok {
		return this._set(v, val)
	}
	return this._set(reflect.ValueOf(src), val)
}

func (this *node) _value(v value, addMissing bool, prev *node) (value, error) {
	var err error

	// fmt.Printf("value: %s\n", this.self.String())
	if this.next != nil {
		v, err = this.next._value(v, addMissing, this)
		if err != nil {
			return v, err
		}
	}
	return this.self.value(v, addMissing, prev)
}

func (this *node) _validate(v reflect.Value) error {

	_, err := this._value(reflectValue(v), false, nil)
	return err
}

func (this *node) _validateType(v reflect.Value, vtype reflect.Type) error {

	field, err := this._value(reflectValue(v), false, nil)
	if err != nil {
		return err
	}
	ftype := field.Type()
	if ftype == vtype {
		return nil
	}

	if ftype.Kind() == reflect.Ptr {
		ftype = ftype.Elem()
		if ftype == vtype {
			return nil
		}
	}
	return fmt.Errorf("%q is not assignable from %q", field.Type(), vtype)
}

func (this *node) _get(v reflect.Value) (interface{}, error) {

	field, err := this._value(reflectValue(v), false, nil)
	if err != nil {
		return nil, err
	}
	return field.Interface(), nil
}

func (this *node) _set(v reflect.Value, val interface{}) error {

	field, err := this._value(reflectValue(v), true, nil)
	if err != nil {
		return err
	}

	a := reflect.ValueOf(val)
	// fmt.Printf("assign %s: %s from %T(%#v)\n", this.self.String(), field.Type(), val, val)

	if val == nil {
		k := v.Kind()
		if k != reflect.Ptr &&
			k != reflect.Slice &&
			k != reflect.Map &&
			k != reflect.Func &&
			k != reflect.Chan &&
			k != reflect.Interface {
			return fmt.Errorf("nil not asignable to %q", v.Type())
		}
		a = reflect.Zero(field.Type())
	} else {
		if field.Kind() == reflect.Ptr && a.Kind() != reflect.Ptr {
			p := reflect.New(a.Type())
			p.Elem().Set(a)
			a = p
		}
		if !a.Type().AssignableTo(field.Type()) {
			return fmt.Errorf("%q not asignable to %q", a.Type(), field.Type())
		}
	}
	field.Set(a)

	return nil
}

func (this *node) toValue(v value, addMissing bool, prev *node) value {
	if v.Kind() == reflect.Interface {
		if !v.IsValid() || v.IsNil() {
			if addMissing {
				// OOPS: some element should be created here, but nobody knows
				// which type to use.
				// try to guess generic intermediate elements by next element
				// in path expression
				switch this.self.(type) {
				case *FieldNode:
					// fmt.Printf("CREATE generic map\n")
					return v.Set(reflect.ValueOf(map[string]interface{}{}))
				case *SliceEntryNode:
					// fmt.Printf("CREATE generic slice\n")
					// keep map entry, but with modified effective type
					return v.Set(reflect.ValueOf([]interface{}{}))
				default:
					return none
				}
			} else {
				// fmt.Print("NIL\n")
				return none
			}
		}
		v = v.Elem()
	}
	if isPtr(v) {
		if v.IsNil() {
			if addMissing {
				// fmt.Printf("CREATE %s\n", v.Type().Elem())
				v.Set(reflect.New(v.Type().Elem()))
			} else {
				// fmt.Print("NIL\n")
				return none
			}
		}
		return v.Elem()
	}
	if v.Kind() == reflect.Map {
		if v.IsNil() {
			if addMissing {
				// fmt.Printf("CREATE %s\n", v.Type().Elem())
				v.Set(reflect.New(v.Type().Elem()))
			} else {
				// fmt.Print("NIL\n")
				return none
			}
		}
		return reflectValue(v.Value())
	}
	return v
}

////////////////////////////////////////////////////////////////////////////////

type FieldNode struct {
	node
	name string
}

var _ Node = &FieldNode{}

func NewFieldNode(name string, next Node) Node {
	f := &FieldNode{name: name}

	return f.new(f, next)
}

func (this *FieldNode) String() string {
	return fmt.Sprintf("%s.%s", this.node.String(), this.name)
}

func (this *FieldNode) value(v value, addMissing bool, prev *node) (value, error) {
	v = this.toValue(v, addMissing, prev)
	if !v.IsValid() {
		return none, fmt.Errorf("%s is <nil>", this.node.String())
	}
	if v.Kind() == reflect.Struct {
		// fmt.Printf("TYPE %s: %s lookup %s\n", this.String(), v.Type(), this.name)
		field := v.Value().FieldByName(this.name)
		if !field.IsValid() {
			return none, fmt.Errorf("%s has no field %q", this.node.String(), this.name)
		}
		return reflectValue(field), nil
	}

	if v.Kind() == reflect.Map {
		if v.Type().Key().Kind() == reflect.String {
			key := reflect.ValueOf(this.name)
			e := v.Value().MapIndex(key)
			if !e.IsValid() {
				if v.Type().Elem().Kind() == reflect.Interface {
					return &mapEntry{v.Value(), key, nil, this.name}, nil
				} else {
					if isSimpleType(v.Type().Elem()) {
						return &mapEntry{v.Value(), key, nil, this.name}, nil
					}
				}
			} else {
				return &mapEntry{v.Value(), key, &e, this.name}, nil
			}
		}
	}
	return none, fmt.Errorf("%s is no struct or string map", this.node.String())
}

////////////////////////////////////////////////////////////////////////////////

type SliceEntryNode struct {
	node
	index int
}

var _ Node = &SliceEntryNode{}

func NewEntry(index int, next Node) Node {
	e := &SliceEntryNode{index: index}
	return e.new(e, next)
}

func (this *SliceEntryNode) String() string {
	return fmt.Sprintf("%s[%d]", this.node.String(), this.index)
}

func (this *SliceEntryNode) value(v value, addMissing bool, prev *node) (value, error) {
	v = this.toValue(v, addMissing, prev)
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return none, fmt.Errorf("%s is no slice or array(%s) ", this.node.String(), v.Type())
	}
	if v.Len() <= this.index {
		if !addMissing || v.Kind() == reflect.Array {
			return none, fmt.Errorf("%s has size %d, but expected at least %d", this.node.String(), v.Len(), this.index+1)
		}
		e := reflect.New(v.Type().Elem())
		for v.Len() <= this.index {
			// fmt.Printf("APPEND %d\n", v.Len())
			v = v.Set(reflect.Append(v.Value(), e.Elem()))
		}
	}
	return reflectValue(v.Value().Index(this.index)), nil
}

////////////////////////////////////////////////////////////////////////////////

type SliceNode struct {
	node
	start int
	end   int
}

var _ Node = &SliceNode{}

func NewSlice(start, end int, next Node) Node {
	e := &SliceNode{start: start, end: end}
	return e.new(e, next)
}

func (this *SliceNode) String() string {
	start := ""
	if this.start > 0 {
		start = strconv.Itoa(this.start)
	}
	end := ""
	if this.end >= 0 {
		end = strconv.Itoa(this.end)
	}
	return fmt.Sprintf("%s[%s:%s]", this.node.String(), start, end)
}

func (this *SliceNode) value(v value, addMissing bool, prev *node) (value, error) {
	v = this.toValue(v, addMissing, prev)
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return none, fmt.Errorf("%s is no slice or array(%s) ", this.node.String(), v.Type())
	}
	end := this.end
	if end < 0 {
		end = v.Len()
		if end < this.start {
			if addMissing {
				end = this.start
			} else {
				return none, fmt.Errorf("%s has size %d, but expected at least %d", this.node.String(), v.Len(), this.start)
			}
		}
	}
	if v.Len() < end {
		if !addMissing || v.Kind() == reflect.Array {
			return none, fmt.Errorf("%s has size %d, but expected at least %d", this.node.String(), v.Len(), end)
		}
		e := reflect.New(v.Type().Elem())
		for v.Len() < end {
			v.Set(reflect.Append(v.Value(), e.Elem()))
		}
	}
	return reflectValue(v.Value().Slice(this.start, end)), nil
}

////////////////////////////////////////////////////////////////////////////////

type SelectionNode struct {
	node
	path  Node
	match interface{}
}

var _ Node = &SelectionNode{}

func NewSelection(path Node, value interface{}, next Node) Node {
	e := &SelectionNode{path: path, match: value}
	return e.new(e, next)
}

func (this *SelectionNode) String() string {
	vs := ""
	switch this.match.(type) {
	case int:
		vs = fmt.Sprintf("%d", this.match)
	case string:
		vs = fmt.Sprintf("%q", this.match)
	}
	return fmt.Sprintf("%s[%s=%s]", this.node.String(), this.path, vs)
}

func (this *SelectionNode) value(v value, addMissing bool, prev *node) (value, error) {
	v = this.toValue(v, addMissing, prev)
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return none, fmt.Errorf("%s is no slice or array(%s) ", this.node.String(), v.Type())
	}
	index := -1
	for i := 0; i < v.Len(); i++ {
		v.Value().Index(i)
		e := this.toValue(reflectValue(v.Value().Index(i)), true, prev)

		match, err := this.path.Get(e)
		if err != nil {
			return none, err
		}
		if match == this.match {
			index = i
		}
	}
	if index < 0 {
		if !addMissing || v.Kind() == reflect.Array {
			return none, fmt.Errorf("no matching element found (%s=%s)", this.path, this.match)
		} else {
			e := reflect.New(v.Type().Elem()).Elem()
			new := this.toValue(reflectValue(e), true, prev)
			err := this.path.Set(new, this.match)
			if err != nil {
				return none, err
			}
			index = v.Len()
			v.Set(reflect.Append(v.Value(), e))
		}
	}
	return reflectValue(v.Value().Index(index)), nil
}

////////////////////////////////////////////////////////////////////////////////

type ProjectionNode struct {
	node
	path Node
}

var _ Node = &ProjectionNode{}

func NewProjection(path Node, next Node) Node {
	e := &ProjectionNode{path: path}
	return e.new(e, next)
}

func (this *ProjectionNode) String() string {
	return fmt.Sprintf("%s[]%s", this.node.String(), this.path)
}

func (this *ProjectionNode) value(v value, addMissing bool, prev *node) (value, error) {
	v = this.toValue(v, addMissing, prev)
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return none, fmt.Errorf("%s is no slice or array(%s) ", this.node.String(), v.Type())
	}
	et, err := this.path.Type(v.Type().Elem())
	if err != nil {
		return none, err
	}
	a := reflect.New(reflect.SliceOf(et)).Elem()
	for i := 0; i < v.Len(); i++ {
		e := this.toValue(reflectValue(v.Value().Index(i)), false, prev)

		if e.Kind() != reflect.Invalid {
			sub, err := this.path._value(e, false, prev)
			if err != nil {
				return none, err
			}
			a = reflect.Append(a, sub.Value())
		}

	}
	return reflectValue(a), nil
}
