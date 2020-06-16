/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved.
 * This file is licensed under the Apache Software License, v. 2 except as noted
 * otherwise in the LICENSE file
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

package abstract

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type AbstractObject struct {
	resource Resource
	ObjectData
}

var _ Object = &AbstractObject{}

func NewAbstractObject(data ObjectData, resource Resource) *AbstractObject {
	return &AbstractObject{resource, data}
}

/////////////////////////////////////////////////////////////////////////////////

func (this *AbstractObject) GetResource() Resource {
	return this.resource
}

func (this *AbstractObject) IsA(spec interface{}) bool {
	switch s := spec.(type) {
	case GroupKindProvider:
		return s.GroupKind() == this.GroupKind()
	case schema.GroupVersionKind:
		return s == this.resource.GroupVersionKind()
	case *schema.GroupVersionKind:
		return *s == this.resource.GroupVersionKind()
	case schema.GroupKind:
		return s == this.GroupKind()
	case *schema.GroupKind:
		return *s == this.GroupKind()
	default:
		t := reflect.TypeOf(s)
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		return reflect.PtrTo(t) == reflect.TypeOf(this.ObjectData)
	}
}

func (this *AbstractObject) Data() ObjectData {
	return this.ObjectData
}

func (this *AbstractObject) Status() interface{} {
	if this.ObjectData == nil {
		return nil
	}
	v := reflect.ValueOf(this.ObjectData).Elem()
	if v.Kind() != reflect.Struct {
		return nil
	}
	f := v.FieldByName("Status")
	if !f.IsValid() {
		return nil
	}
	if f.Kind() == reflect.Ptr {
		return f.Interface()
	}
	if !f.CanAddr() {
		return nil
	}
	return f.Addr().Interface()
}

func (this *AbstractObject) ObjectName() ObjectName {
	return NewObjectName(this.GetNamespace(), this.GetName())
}

func (this *AbstractObject) Key() ObjectKey {
	return NewKey(this.GroupKind(), this.GetNamespace(), this.GetName())
}

func (this *AbstractObject) Description() string {
	return fmt.Sprintf("%s", this.Key())
}

func (this *AbstractObject) GroupKind() schema.GroupKind {
	return this.resource.GroupKind()
}

func (this *AbstractObject) GroupVersionKind() schema.GroupVersionKind {
	return this.resource.GroupVersionKind()
}
