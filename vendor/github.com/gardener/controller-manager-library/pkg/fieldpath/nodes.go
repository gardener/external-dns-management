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

	"github.com/gardener/controller-manager-library/pkg/utils"
)

type Node interface {
	Next() Node
	String() string

	Type(interface{}) (reflect.Type, error)
	VType(interface{}) (reflect.Type, error)
	Validate(interface{}) error
	ValidateType(interface{}, interface{}) error

	Get(interface{}) (interface{}, error)
	Set(interface{}, interface{}) error

	_value(src value, addMissing bool, prev *node) (value, error)
	value(src value, addMissing bool, prev *node) (value, error)

	_vtype(t reflect.Type, src value, prev *node) (reflect.Type, value, error)
	vtype(t reflect.Type, src value, prev *node) (reflect.Type, value, error)
}

////////////////////////////////////////////////////////////////////////////////

type new interface {
	new(self, next Node) Node
}

type node struct {
	context Node
	next    Node
	self    Node
}

func (this *node) new(ctx, self, next Node) Node {
	this.context = ctx
	this.self = self
	this.next = next
	return self
}

func (this *node) Next() Node {
	return this.next
}

func (this *node) String() string {
	if this.next == nil {
		if this.context != nil {
			return this.context.String()
		}
		return "<object>"
	}
	return this.next.String()
}

func (this *node) Type(src interface{}) (reflect.Type, error) {
	v, ok := src.(reflect.Value)
	if ok {
		if v.Kind() == reflect.Interface {
			return TInterface, nil
		}
		return this._type(v)
	}
	t, ok := src.(reflect.Type)
	if ok {
		if t.Kind() == reflect.Interface {
			return TInterface, nil
		}
		return this._type(reflect.New(t))
	}
	return this._type(reflect.ValueOf(src))
}

func (this *node) VType(src interface{}) (reflect.Type, error) {
	if src == nil {
		return nil, fmt.Errorf("unexpected nil")
	}
	var v value
	t, ok := src.(reflect.Type)
	if ok {
		if t.Kind() == reflect.Interface {
			return TInterface, nil
		}
	}
	if !ok {
		vv, ok := src.(reflect.Value)
		if ok {
			if vv.Kind() == reflect.Interface {
				return TInterface, nil
			}
			v = reflectValue(vv)
		} else {
			v = reflectValue(reflect.ValueOf(src))
		}
		t = v.Type()
	}

	t, _, err := this._vtype(t, v, nil)
	return t, err
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

func (this *node) _vtype(t reflect.Type, v value, prev *node) (reflect.Type, value, error) {
	var err error

	// fmt.Printf("value: %s\n", this.self.String())
	if this.next != nil {
		t, v, err = this.next._vtype(t, v, this)
		if err != nil || t == Unknown {
			return t, v, err
		}
		if v == nil && t == TInterface {
			return Unknown, v, err
		}
	}
	return this.self.vtype(t, v, prev)
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

type MAP = map[string]interface{}

var TMap = reflect.TypeOf(MAP{})

type ARRAY = []interface{}

var TArray = reflect.TypeOf(ARRAY{})

var anyvalue *interface{}
var TInterface = reflect.TypeOf(anyvalue).Elem()

func (this *node) toValue(v value, addMissing bool, prev *node) (value, bool) {
	if v == nil {
		return nil, true
	}
	nil := false
	if v.Kind() == reflect.Interface {
		nil = !v.IsValid() || v.IsNil()
		if nil {
			if addMissing {
				// OOPS: some element should be created here, but nobody knows
				// which type to use.
				// try to guess generic intermediate elements by next element
				// in path expression
				switch this.self.(type) {
				case *FieldNode:
					// fmt.Printf("CREATE dynamic map\n")
					v = v.Set(reflect.ValueOf(MAP{}))
					if v.Value().Kind() == reflect.Interface {
						return v.Elem(), nil
					}
					return v, nil
				case *SliceEntryNode, *SliceNode:
					// fmt.Printf("CREATE dynamic slice\n")
					// keep map entry, but with modified effective type
					v = v.Set(reflect.ValueOf(ARRAY{}))
					if v.Value().Kind() == reflect.Interface {
						e := v.Elem().Value()
						return interfaceValue{v.Value(), &e}, nil
					}
					return v, nil
				default:
					return none, nil
				}
			} else {
				// fmt.Print("NIL\n")
				return none, nil
			}
		}
		v = v.Elem()
	}
	if IsPtr(v) {
		nil = v.IsNil()
		if nil {
			if addMissing {
				// fmt.Printf("CREATE %s\n", v.Type().Elem())
				v.Set(reflect.New(v.Type().Elem()))
			} else {
				// fmt.Print("NIL\n")
				return none, nil
			}
		}
		//return v.Elem(), nil
		v, unset := this.toValue(v.Elem(), addMissing, prev)
		if nil {
			return v, nil
		}
		return v, unset
	}
	if v.Kind() == reflect.Map {
		nil = v.IsNil()
		if nil {
			if addMissing {
				// fmt.Printf("CREATE %s\n", v.Type().Elem())
				v.Set(reflect.New(v.Type().Elem()))
			} else {
				// fmt.Print("NIL\n")
				return none, nil
			}
		}
		return reflectValue(v.Value()), nil
	}
	return v, nil
}

func (this *node) toType(t reflect.Type, v value, prev *node) (reflect.Type, value) {
	v, _ = this.toValue(v, false, prev)
	if v != nil && !v.IsValid() {
		v = nil
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Interface {
		// OOPS: some element should be created here, but nobody knows
		// which type to use.
		// try to guess generic intermediate elements by next element
		// in path expression
		// fmt.Printf("dynamic map\n")
		// fmt.Printf("CREATE dynamic slice\n")
		// keep map entry, but with modified effective type
		switch this.self.(type) {
		case *FieldNode:
			return TMap, v
		case *SliceEntryNode, *SliceNode:
			return TArray, v
		default:
			return Unknown, nil
		}
	}
	return t, v
}

////////////////////////////////////////////////////////////////////////////////

type FieldNode struct {
	node
	name string
}

var _ Node = &FieldNode{}

func NewFieldNode(ctx Node, name string, next Node) Node {
	f := &FieldNode{name: name}

	return f.new(ctx, f, next)
}

func (this *FieldNode) String() string {
	return fmt.Sprintf("%s.%s", this.node.String(), this.name)
}

func (this *FieldNode) value(v value, addMissing bool, prev *node) (value, error) {
	var unset bool
	v, unset = this.toValue(v, true, prev) // TODO: addMissing
	if !v.IsValid() {
		if unset {
			return reflectValue(reflect.ValueOf(Unknown)), nil
		}
		return none, fmt.Errorf("%s is <nil>", this.node.String())
	}
	if v.Kind() == reflect.Struct {
		// fmt.Printf("TYPE %s: %s lookup %s\n", this.String(), v.Type(), this.name)
		field := v.Value().FieldByName(this.name)
		if !field.IsValid() {
			return none, fmt.Errorf("%s has no field %q", this.node.String(), this.name)
		}
		if unset && !addMissing {
			field = reflect.ValueOf(Unknown)
		}
		return reflectValue(field), nil
	}

	if v.Kind() == reflect.Map {
		if unset && !addMissing {
			return reflectValue(reflect.ValueOf(Unknown)), nil
		}
		if v.Type().Key().Kind() == reflect.String {
			key := reflect.ValueOf(this.name)
			e := v.Value().MapIndex(key)
			if !e.IsValid() {
				if v.Type().Elem().Kind() == reflect.Interface {
					return &mapEntry{v.Value(), key, nil, this.name}, nil
				} else {
					if IsSimpleType(v.Type().Elem()) {
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

func (this *FieldNode) vtype(t reflect.Type, v value, prev *node) (reflect.Type, value, error) {
	t, v = this.toType(t, v, prev)
	if t == Unknown {
		return t, v, nil
	}
	if t.Kind() == reflect.Struct {
		// fmt.Printf("TYPE %s: %s lookup %s\n", this.String(), v.Type(), this.name)
		field, ok := t.FieldByName(this.name)
		if !ok {
			return nil, nil, fmt.Errorf("%s (%T) has no field %q", this.node.String(), t, this.name)
		}
		if v != nil {
			fieldv := v.Value().FieldByName(this.name)
			if !fieldv.IsValid() {
				panic("mismatch value and type")
			}
			v = reflectValue(fieldv)
		} else {
			v = nil
		}
		return field.Type, v, nil
	}

	if t.Kind() == reflect.Map {
		if t.Key().Kind() == reflect.String {
			if v != nil && v.IsValid() {
				key := reflect.ValueOf(this.name)
				e := v.Value().MapIndex(key)
				if e.IsValid() {
					return e.Type(), reflectValue(e), nil
				}
			}
			return t.Elem(), nil, nil
		}
	}
	return nil, nil, fmt.Errorf("%s is no struct or string map", this.node.String())
}

////////////////////////////////////////////////////////////////////////////////

type SliceEntryBase struct {
	node
	index int
}

func (this *SliceEntryBase) vtype(t reflect.Type, v value, prev *node) (reflect.Type, value, error) {
	t, v = this.toType(t, v, prev)
	if t.Kind() != reflect.Array && t.Kind() != reflect.Slice {
		return nil, nil, fmt.Errorf("%s is no slice or array(%s) ", this.node.String(), t)
	}
	index := this.index
	if v.Value().Len() <= this.index {
		index = 0
	}
	if v.Value().Len() > index {
		v = reflectValue(v.Value().Index(index))
	} else {
		v = nil
	}
	return t.Elem(), v, nil
}

////////////////////////////////////////////////////////////////////////////////

type SliceEntryNode struct {
	SliceEntryBase
}

var _ Node = &SliceEntryNode{}

func NewEntry(ctx Node, index int, next Node) Node {
	e := &SliceEntryNode{SliceEntryBase{index: index}}
	return e.new(ctx, e, next)
}

func (this *SliceEntryNode) String() string {
	return fmt.Sprintf("%s[%d]", this.node.String(), this.index)
}

func (this *SliceEntryNode) value(v value, addMissing bool, prev *node) (value, error) {
	v, _ = this.toValue(v, addMissing, prev)
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return none, fmt.Errorf("%s is no slice or array(%s) ", this.node.String(), v.Type())
	}
	index := this.index
	if this.index == -1 {
		index = v.Len()
	}
	if v.Len() <= index {
		if !addMissing || v.Kind() == reflect.Array {
			return none, fmt.Errorf("%s has size %d, but expected at least %d", this.node.String(), v.Len(), this.index+1)
		}
		e := reflect.New(v.Type().Elem())
		for v.Len() <= index {
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

func NewSlice(ctx Node, start, end int, next Node) Node {
	e := &SliceNode{start: start, end: end}
	return e.new(ctx, e, next)
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
	if start == "" && end == "" {
		return fmt.Sprintf("%s[]", this.node.String())
	}
	return fmt.Sprintf("%s[%s:%s]", this.node.String(), start, end)
}

func (this *SliceNode) value(v value, addMissing bool, prev *node) (value, error) {
	v, _ = this.toValue(v, addMissing, prev)
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return none, fmt.Errorf("%s is no slice or array(%s) ", this.node.String(), v.Type())
	}
	end := this.end
	if end < 0 {
		if this.start < 0 {
			return v, nil
		}
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

func (this *SliceNode) vtype(t reflect.Type, v value, prev *node) (reflect.Type, value, error) {
	t, v = this.toType(t, v, prev)
	if t.Kind() != reflect.Array && t.Kind() != reflect.Slice {
		return nil, nil, fmt.Errorf("%s is no slice or array(%s) ", this.node.String(), t)
	}
	return t, v, nil
}

////////////////////////////////////////////////////////////////////////////////

type SelectionNode struct {
	SliceEntryBase
	path  Node
	match interface{}
}

var _ Node = &SelectionNode{}

func NewSelection(ctx, path Node, value interface{}, next Node) Node {
	e := &SelectionNode{path: path, match: value}
	return e.new(ctx, e, next)
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
	v, _ = this.toValue(v, addMissing, prev)
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return none, fmt.Errorf("%s is no slice or array(%s) ", this.node.String(), v.Type())
	}
	index := -1
	for i := 0; i < v.Len(); i++ {
		v.Value().Index(i)
		e, _ := this.toValue(reflectValue(v.Value().Index(i)), true, prev)

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
			new, _ := this.toValue(reflectValue(e), true, prev)
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

func NewProjection(ctx Node, path Node, next Node) Node {
	e := &ProjectionNode{path: path}
	return e.new(ctx, e, next)
}

func (this *ProjectionNode) String() string {
	return fmt.Sprintf("%s[]%s", this.node.String(), this.path)
}

func (this *ProjectionNode) value(v value, addMissing bool, prev *node) (value, error) {
	o := v
	v, _ = this.toValue(v, addMissing, prev)
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return none, fmt.Errorf("%s is no slice or array(%s) ", this.node.String(), o.Type())
	}
	et, err := this.path.Type(v.Type().Elem())
	if err != nil {
		return none, err
	}
	ifce := v.Type().Elem().Kind() == reflect.Interface
	a := reflect.New(reflect.SliceOf(et)).Elem()
	for i := 0; i < v.Len(); i++ {
		en := v.Value().Index(i)
		if !utils.IsNil(en) {
			e, _ := this.toValue(reflectValue(en), false, prev)

			if e.Kind() != reflect.Invalid {
				sub, err := this.path._value(e, false, prev)
				if err != nil {
					if ifce {
						continue
					}
					return none, err
				}
				if sub.Interface() == Unknown {
					continue
				}
				a = reflect.Append(a, sub.Value())
			}
		}
	}
	return reflectValue(a), nil
}

func (this *ProjectionNode) vtype(t reflect.Type, v value, prev *node) (reflect.Type, value, error) {
	t, v = this.toType(t, v, prev)

	if t.Kind() != reflect.Array && t.Kind() != reflect.Slice {
		return nil, nil, fmt.Errorf("%s is no slice or array(%s) ", this.node.String(), t)
	}
	et, err := this.path.VType(t.Elem())
	if err != nil {
		return nil, nil, err
	}
	if et == Unknown {
		return TArray, nil, nil
	}
	return reflect.SliceOf(et), nil, nil
}
