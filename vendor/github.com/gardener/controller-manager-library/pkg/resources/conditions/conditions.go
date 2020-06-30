/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package conditions

import (
	"fmt"
	"reflect"
	"time"
	"unsafe"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

type ModificationHandler interface {
	Modified(*Condition)
}

type ModificationHandlers []ModificationHandler

////////////////////////////////////////////////////////////////////////////////

// Condition reflects a dedicated condition for a dedicated object. It can
// be retrieved for a dedicated object using a ConditionType object.
type Condition struct {
	otype    reflect.Type
	ctype    *ConditionType
	conds    *reflect.Value
	cond     *reflect.Value
	modified bool
	handlers ModificationHandlers
}

func newCondition(o interface{}, ctype *ConditionType, conds *reflect.Value, cond *reflect.Value) *Condition {
	return &Condition{reflect.TypeOf(o), ctype, conds, cond, false, ModificationHandlers{}}
}

func (this *Condition) Name() string {
	return this.ctype.Name()
}

func (this *Condition) IsModified() bool {
	return this.modified
}

func (this *Condition) ResetModified() {
	this.modified = false
}

func (this *Condition) Modify(m bool) {
	if m {
		this.modify()
	}
}

func (this *Condition) AddModificationHandler(h ModificationHandler) {
	this.handlers = append(this.handlers, h)
	if this.modified {
		h.Modified(this)
	}
}

func (this *Condition) RemoveModificationHandler(h ModificationHandler) {
	for i, e := range this.handlers {
		if e == h {
			this.handlers = append(this.handlers[:i], this.handlers[i+1:]...)
			return
		}
	}
}

func (this *Condition) modify() {
	this.modified = true
	for _, h := range this.handlers {
		h.Modified(this)
	}
}

func (this *Condition) Interface() interface{} {
	if this == nil || this.cond == nil {
		return nil
	}
	return this.cond.Addr().Interface()
}

func (this *Condition) Has() bool {
	return this != nil && this.cond != nil
}

func (this *Condition) Delete() (bool, error) {
	if this == nil || this.conds == nil {
		return false, fmt.Errorf("no conditions fields in %s", this.otype)
	}
	if this.conds.IsNil() {
		return false, nil
	}
	mod := this.ctype._delete(this.conds)
	this.Modify(mod)
	return mod, nil
}

func (this *Condition) Assure() error {
	if this == nil || this.conds == nil {
		return fmt.Errorf("no conditions fields in %s", this.otype)
	}
	if this.cond != nil {
		return nil
	}
	if this.conds.IsNil() {
		v := reflect.New(this.conds.Type())
		this.conds.Set(v.Elem())
	}

	v := reflect.New(this.conds.Type().Elem())
	t := v.Elem().FieldByName(this.ctype.cTypeField)

	err := utils.SetValue(t, this.ctype.name)
	if err != nil {
		return fmt.Errorf("cannot set type value for new condition %s in %s: %s", this.ctype.name, this.otype, err)
	}
	this.conds.Set(reflect.Append(*this.conds, v.Elem()))

	v = this.conds.Index(this.conds.Len() - 1)
	this.cond = &v
	this.modify()
	return nil
}

func (this *Condition) AssureInterface() interface{} {
	this.Assure()
	return this.Interface()
}

func (this *Condition) Get(name string) interface{} {
	if this.cond == nil {
		return nil
	}
	f := this.cond.FieldByName(name)
	if f.Kind() == reflect.Invalid {
		return nil
	}
	return f.Interface()
}

func (this *Condition) set(name string, value interface{}) (bool, error) {
	if name == "" {
		return false, fmt.Errorf("field not defined for conditions of %s", this.otype)
	}
	err := this.Assure()
	if err != nil {
		return false, err
	}
	v := this.cond

	f := v.FieldByName(name)
	if f.Kind() == reflect.Invalid {
		return false, fmt.Errorf("field %s not found in conditions of %s", name, this.otype)
	}
	vv := reflect.ValueOf(value)
	if f.Type() != vv.Type() {
		if vv.Type().ConvertibleTo(f.Type()) {
			vv = vv.Convert(f.Type())
		} else {
			if f.Kind() == reflect.Struct && f.NumField() == 1 && f.Field(0).Type() == vv.Type() {
				// handle simple wrapped fields like metav1.Time
				tmp := reflect.New(f.Type()).Elem()
				f := tmp.Field(0)
				if !f.CanSet() {
					f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem() // yepp, access unexported fields
				}
				f.Set(vv)
				vv = tmp
			} else {
				return false, fmt.Errorf("invalid type (%s) for field %s in conditions of %s (expected %s)",
					vv.Type(), name, this.otype, f.Type())
			}
		}
	}
	if !f.CanSet() {
		f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem() // yepp, access unexported fields
	}
	old := f.Interface()
	if !reflect.DeepEqual(old, value) {
		this.modify()
		f.Set(vv)
		return true, nil
	}
	return false, nil
}

func (this *Condition) Set(name string, value interface{}) error {
	if this == nil {
		return fmt.Errorf("no conditions")
	}
	mod, err := this.set(name, value)
	if err != nil {
		return err
	}
	if mod {
		var now time.Time
		if name == this.ctype.cStatusField && this.ctype.cTransitionField != "" {
			now = time.Now()
			this.set(this.ctype.cTransitionField, now)
		}
		if name != this.ctype.cUpdateField && this.ctype.cUpdateField != "" {
			if now.IsZero() {
				now = time.Now()
			}
			this.set(this.ctype.cUpdateField, now)
		}
	}
	return nil
}

//////////

func (this *Condition) GetStringField(name string) string {
	if this == nil || name == "" {
		return ""
	}
	v := this.Get(name)
	if v == nil {
		return ""
	}
	return v.(string)
}

func (this *Condition) GetTimeField(name string) time.Time {
	if this == nil || name == "" {
		return time.Time{}
	}
	v := this.Get(name)
	if v == nil {
		return time.Time{}
	}
	return v.(time.Time)
}

func (this *Condition) GetMessage() string {
	return this.GetStringField(this.ctype.cMessageField)
}

func (this *Condition) GetStatus() string {
	return this.GetStringField(this.ctype.cStatusField)
}

func (this *Condition) GetReason() string {
	return this.GetStringField(this.ctype.cMessageField)
}

func (this *Condition) GetTransitionTime() time.Time {
	return this.GetTimeField(this.ctype.cTransitionField)
}

func (this *Condition) GetLastUpdateTime() time.Time {
	return this.GetTimeField(this.ctype.cUpdateField)
}

//////////

func (this *Condition) SetMessage(v string) error {
	if this.ctype.cMessageField == "" {
		return fmt.Errorf("message field not defined for conditions of %s", this.otype)
	}
	return this.Set(this.ctype.cMessageField, v)
}

func (this *Condition) SetStatus(v string) error {
	if this.ctype.cStatusField == "" {
		return fmt.Errorf("status field not defined for conditions of %s", this.otype)
	}
	return this.Set(this.ctype.cStatusField, v)
}

func (this *Condition) SetReason(v string) error {
	if this.ctype.cReasonField == "" {
		return fmt.Errorf("reason field not defined for conditions of %s", this.otype)
	}
	return this.Set(this.ctype.cReasonField, v)
}

func (this *Condition) SetTransitionTime(v time.Time) error {
	if this.ctype.cTransitionField == "" {
		return fmt.Errorf("transition time field not defined for conditions of %s", this.otype)
	}
	return this.Set(this.ctype.cTransitionField, v)
}

func (this *Condition) SetLastUpdateTime(v time.Time) error {
	if this.ctype.cUpdateField == "" {
		return fmt.Errorf("last update time field not defined for conditions of %s", this.otype)
	}
	return this.Set(this.ctype.cUpdateField, v)
}

////////////////////////////////////////////////////////////////////////////////

// ConditionLayout represents a dedicated kind of condition layout for a
// dedicated class of condition carrying objects. Therefore it holds field name
// information to to access conditions in this class of objects and
// about the representation of some standard field like Type and Status
// inside a condition entry.
// A ConditionType is configured using
// an arbitrary set of TweakFunctions. There are dedicated creator
// functions for all modifyable attributes.
type ConditionLayout struct {
	statusField     string
	conditionsField string

	cTypeField       string
	cMessageField    string
	cStatusField     string
	cReasonField     string
	cTransitionField string
	cUpdateField     string
}

func NewConditionLayout(cfg ...TweakFunction) *ConditionLayout {
	c := &ConditionLayout{
		statusField:     "Status",
		conditionsField: "Conditions",

		cTypeField:       "Type",
		cMessageField:    "Message",
		cStatusField:     "Status",
		cReasonField:     "Reason",
		cTransitionField: "TransitionTime",
		cUpdateField:     "LastUpdateTime",
	}
	for _, f := range cfg {
		f(c)
	}
	return c
}

func (this *ConditionLayout) For(o interface{}) (*Conditions, error) {
	return newConditions(this, o)
}

func (this *ConditionLayout) conditions(o interface{}) *reflect.Value {
	v := reflect.ValueOf(o)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() == reflect.Struct {
		f := v.FieldByName(this.statusField)
		if f.Kind() != reflect.Invalid {
			v = f
		}
	}
	v = v.FieldByName(this.conditionsField)
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return nil
	}
	return &v
}

func (this *ConditionLayout) Types(o interface{}) utils.StringSet {
	v := this.conditions(o)
	if v == nil {
		return nil
	}
	return this._types(v)
}

func (this *ConditionLayout) _types(conds *reflect.Value) utils.StringSet {
	set := utils.StringSet{}
	for i := 0; i < conds.Len(); i++ {
		c := conds.Index(i)
		if c.Kind() == reflect.Struct {
			f := c.FieldByName(this.cTypeField)
			if f.Kind() == reflect.String {
				set.Add(f.String())
			}
		}
	}
	return set
}

////////////////////////////////////////////////////////////////////////////////

// ConditionType represents a dedicated kind of condition for a dedicated
// class of condition carrying objects using a dedicated condition layout.
// Therefore is is configured by a name for the dedicated condition kind
// a condition layout.
type ConditionType struct {
	name string
	*ConditionLayout
}

var defaultLayout = NewConditionLayout()

func NewConditionType(name string, t *ConditionLayout) *ConditionType {
	if t == nil {
		t = defaultLayout
	}
	c := &ConditionType{
		name:            name,
		ConditionLayout: t,
	}
	return c
}

func (this *ConditionType) Name() string {
	return this.name
}

func (this *ConditionType) get(o interface{}) *Condition {
	return this._get(o, this.conditions(o))
}

func (this *ConditionType) _get(o interface{}, conds *reflect.Value) *Condition {
	if conds == nil {
		return nil
	}
	for i := 0; i < conds.Len(); i++ {
		c := conds.Index(i)
		if c.Kind() == reflect.Struct {
			f := c.FieldByName(this.cTypeField)
			if f.Kind() == reflect.String {
				if f.String() == this.name {
					return newCondition(o, this, conds, &c)
				}
			}
		}
	}
	return newCondition(o, this, conds, nil)
}

func (this *ConditionType) _delete(conds *reflect.Value) bool {
	if conds == nil {
		return false
	}
	for i := 0; i < conds.Len(); i++ {
		c := conds.Index(i)
		if c.Kind() == reflect.Struct {
			f := c.FieldByName(this.cTypeField)
			if f.Kind() == reflect.String {
				if f.String() == this.name {
					conds.Set(reflect.AppendSlice(conds.Slice(0, i), conds.Slice(i+1, conds.Len())))
					return true
				}
			}
		}
	}
	return false
}

func (this *ConditionType) Has(o interface{}) bool {
	return this.get(o).Has()
}

func (this *ConditionType) GetInterface(o interface{}) interface{} {
	return this.get(o).Interface()
}

func (this *ConditionType) Get(o interface{}) *Condition {
	return this.get(o)
}

func (this *ConditionType) DeleteCondition(o interface{}) bool {
	conds := this.conditions(o)
	if conds == nil {
		return false
	}
	return this._delete(conds)
}

func (this *ConditionType) AssureInterface(o interface{}) interface{} {
	return this.Assure(o).Interface()
}

func (this *ConditionType) Assure(o interface{}) *Condition {
	c := this.get(o)
	if c.Has() {
		return c
	}
	if c == nil {
		return nil
	}
	c.Assure()
	return c
}

func (this *ConditionType) SetValue(o interface{}, name string, value interface{}) error {
	c := this.Get(o)
	if c == nil {
		return fmt.Errorf("no conditions for %s", reflect.TypeOf(o))
	}
	return c.Set(name, value)
}

func (this *ConditionType) SetMessage(o interface{}, v string) error {
	if this.cMessageField == "" {
		return fmt.Errorf("message field not defined for conditions of %s", reflect.TypeOf(o))
	}
	return this.SetValue(o, this.cMessageField, v)
}

func (this *ConditionType) SetStatus(o interface{}, v string) error {
	if this.cStatusField == "" {
		return fmt.Errorf("status field not defined for conditions of %s", reflect.TypeOf(o))
	}
	return this.SetValue(o, this.cStatusField, v)
}

func (this *ConditionType) SetReason(o interface{}, v string) error {
	if this.cReasonField == "" {
		return fmt.Errorf("reason field not defined for conditions of %s", reflect.TypeOf(o))
	}
	return this.SetValue(o, this.cReasonField, v)
}

func (this *ConditionType) SetTransitionTime(o interface{}, v time.Time) error {
	if this.cTransitionField == "" {
		return fmt.Errorf("transition time field not defined for conditions of %s", reflect.TypeOf(o))
	}
	return this.SetValue(o, this.cTransitionField, v)
}

func (this *ConditionType) SetLastUpdateTime(o interface{}, v time.Time) error {
	if this.cUpdateField == "" {
		return fmt.Errorf("last update time field not defined for conditions of %s", reflect.TypeOf(o))
	}
	return this.SetValue(o, this.cUpdateField, v)
}

////////

func (this *ConditionType) GetValue(o interface{}, name string) interface{} {
	c := this.Get(o)
	if c == nil {
		return nil
	}
	return c.Get(name)
}

func (this *ConditionType) GetStringField(o interface{}, name string) string {
	if name == "" {
		return ""
	}
	return this.Get(o).GetStringField(name)
}

func (this *ConditionType) GetTimeField(o interface{}, name string) time.Time {
	if name == "" {
		return time.Time{}
	}
	return this.Get(o).GetTimeField(name)
}

func (this *ConditionType) GetMessage(o interface{}) string {
	return this.GetStringField(o, this.cMessageField)
}

func (this *ConditionType) GetStatus(o interface{}) string {
	return this.GetStringField(o, this.cStatusField)
}

func (this *ConditionType) GetReason(o interface{}) string {
	return this.GetStringField(o, this.cReasonField)
}

func (this *ConditionType) GetTransitionTime(o interface{}) time.Time {
	return this.GetTimeField(o, this.cTransitionField)
}

func (this *ConditionType) GetLastUpdateTime(o interface{}) time.Time {
	return this.GetTimeField(o, this.cUpdateField)
}

////////////////////////////////////////////////////////////////////////////////

type Conditions struct {
	layout     *ConditionLayout
	object     interface{}
	conds      *reflect.Value
	conditions map[string]*Condition
	modified   bool
	handler    ModificationHandler
	handlers   ModificationHandlers
}

type modhandler struct {
	conditions *Conditions
}

func (this *modhandler) Modified(c *Condition) {
	this.conditions.modify(c)
}

func newConditions(layout *ConditionLayout, o interface{}) (*Conditions, error) {
	conds := layout.conditions(o)
	if conds == nil {
		return nil, fmt.Errorf("no conditions field %q in %T", layout.conditionsField, reflect.TypeOf(o))
	}
	c := &Conditions{layout, o, conds, map[string]*Condition{}, false, nil, nil}
	c.handler = &modhandler{c}
	return c, nil
}

func (this *Conditions) IsModified() bool {
	return this.modified
}

func (this *Conditions) ResetModified() bool {
	defer func() { this.modified = false }()
	return this.modified
}

func (this *Conditions) AddModificationHandler(h ModificationHandler) {
	this.handlers = append(this.handlers, h)
	if this.modified {
		h.Modified(nil)
	}
}

func (this *Conditions) modify(c *Condition) {
	this.modified = true
	for _, h := range this.handlers {
		h.Modified(c)
	}
}

func (this *Conditions) RemoveModificationHandler(h ModificationHandler) {
	for i, e := range this.handlers {
		if e == h {
			this.handlers = append(this.handlers[:i], this.handlers[i+1:]...)
			return
		}
	}
}

func (this *Conditions) Types() utils.StringSet {
	return this.layout._types(this.conds)
}

func (this *Conditions) Get(name string) *Condition {
	if c, ok := this.conditions[name]; ok {
		return c
	}
	c := NewConditionType(name, this.layout)._get(this.object, this.conds)
	c.AddModificationHandler(this.handler)
	this.conditions[name] = c
	return c
}

func (this *Conditions) Delete(name string) bool {
	c, ok := this.conditions[name]
	if !ok {
		c := NewConditionType(name, this.layout)._get(this.object, this.conds)
		c.AddModificationHandler(this.handler)
		this.conditions[name] = c
	}
	mod, _ := c.Delete()
	return mod
}

func (this *Conditions) Modify(m bool) bool {
	if !this.modified && m {
		defer this.modify(nil)
	}
	this.modified = this.modified || m
	return this.modified
}

func (this *Conditions) SetValue(ctype, name string, value interface{}) error {
	c := this.Get(ctype)
	return c.Set(name, value)
}

func (this *Conditions) SetMessage(ctype, v string) error {
	c := this.Get(ctype)
	return c.SetMessage(v)
}

func (this *Conditions) SetStatus(ctype, v string) error {
	c := this.Get(ctype)
	return c.SetStatus(v)
}

func (this *Conditions) SetReason(ctype, v string) error {
	c := this.Get(ctype)
	return c.SetReason(v)
}

func (this *Conditions) SetTransitionTime(ctype string, v time.Time) error {
	c := this.Get(ctype)
	return c.SetTransitionTime(v)
}

func (this *Conditions) SetLastUpdateTime(ctype string, v time.Time) error {
	c := this.Get(ctype)
	return c.SetLastUpdateTime(v)
}

////////

func (this *Conditions) GetValue(ctype, name string) interface{} {
	c := this.Get(ctype)
	return c.Get(name)
}

func (this *Conditions) GetStringField(ctype, name string) string {
	if name == "" {
		return ""
	}
	return this.Get(ctype).GetStringField(name)
}

func (this *Conditions) GetTimeField(ctype string, name string) time.Time {
	if name == "" {
		return time.Time{}
	}
	return this.Get(ctype).GetTimeField(name)
}

func (this *Conditions) GetMessage(ctype string) string {
	c := this.Get(ctype)
	return c.GetMessage()
}

func (this *Conditions) GetStatus(ctype string) string {
	c := this.Get(ctype)
	return c.GetStatus()
}

func (this *Conditions) GetReason(ctype string) string {
	c := this.Get(ctype)
	return c.GetReason()
}

func (this *Conditions) GetTransitionTime(ctype string) time.Time {
	c := this.Get(ctype)
	return c.GetTransitionTime()
}

func (this *Conditions) GetLastUpdateTime(ctype string) time.Time {
	c := this.Get(ctype)
	return c.GetLastUpdateTime()
}

////////////////////////////////////////////////////////////////////////////////

// TweakFunction is used to configure a ConditionType for a dedicated
// class of objects
type TweakFunction func(c *ConditionLayout)

func Inherit(b *ConditionLayout) TweakFunction {
	return func(this *ConditionLayout) {
		*this = *b
	}
}
func ObjectStatusField(field string) TweakFunction {
	return func(this *ConditionLayout) {
		this.statusField = field
	}
}
func ConditionsField(field string) TweakFunction {
	return func(this *ConditionLayout) {
		this.conditionsField = field
	}
}
func TypeField(field string) TweakFunction {
	return func(this *ConditionLayout) {
		this.cTypeField = field
	}
}
func StatusField(field string) TweakFunction {
	return func(this *ConditionLayout) {
		this.cStatusField = field
	}
}
func MessageField(field string) TweakFunction {
	return func(this *ConditionLayout) {
		this.cMessageField = field
	}
}
func ReasonField(field string) TweakFunction {
	return func(this *ConditionLayout) {
		this.cReasonField = field
	}
}
func TransitionTimeField(field string) TweakFunction {
	return func(this *ConditionLayout) {
		this.cTransitionField = field
	}
}
func LastUpdateTimeField(field string) TweakFunction {
	return func(this *ConditionLayout) {
		this.cUpdateField = field
	}
}
