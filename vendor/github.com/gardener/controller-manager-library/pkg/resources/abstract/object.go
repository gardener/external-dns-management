/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package abstract

import (
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type AbstractObject struct {
	resource Resource
	ObjectData
	minimal bool
}

var _ Object = &AbstractObject{}

func NewAbstractObject(data ObjectData, resource Resource) *AbstractObject {
	_, minimal := data.(*metav1.PartialObjectMetadata)
	return &AbstractObject{resource: resource, ObjectData: data, minimal: minimal}
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

func (this *AbstractObject) MinimalData() *metav1.PartialObjectMetadata {
	if !this.minimal {
		return nil
	}
	return this.ObjectData.(*metav1.PartialObjectMetadata)
}

func (this *AbstractObject) IsMinimal() bool {
	return this.minimal
}

func (this *AbstractObject) StatusField() interface{} {
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
