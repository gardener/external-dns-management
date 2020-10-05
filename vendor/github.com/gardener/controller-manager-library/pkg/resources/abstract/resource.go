/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package abstract

import (
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/controller-manager-library/pkg/resources/errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type AbstractResource struct {
	context ResourceContext
	gvk     schema.GroupVersionKind
	otype   reflect.Type
	ltype   reflect.Type
}

func NewAbstractResource(
	context ResourceContext,
	otype reflect.Type,
	ltype reflect.Type,
	gvk schema.GroupVersionKind) *AbstractResource {

	return &AbstractResource{
		context: context,
		gvk:     gvk,
		otype:   otype,
		ltype:   ltype,
	}
}

/////////////////////////////////////////////////////////////////////////////////

func (this *AbstractResource) IsUnstructured() bool {
	return this.otype == unstructuredType
}

func (this *AbstractResource) ResourceContext() ResourceContext {
	return this.context
}

func (this *AbstractResource) ObjectType() reflect.Type {
	return this.otype
}

func (this *AbstractResource) ListType() reflect.Type {
	return this.ltype
}

func (this *AbstractResource) GroupVersionKind() schema.GroupVersionKind {
	return this.gvk
}

func (this *AbstractResource) GroupKind() schema.GroupKind {
	return this.gvk.GroupKind()
}

func (this *AbstractResource) New(name ObjectName) ObjectData {
	data := this.CreateData()
	data.GetObjectKind().SetGroupVersionKind(this.GroupVersionKind())
	if name != nil {
		data.SetName(name.Name())
		data.SetNamespace(name.Namespace())
	}
	return data
}

func (this *AbstractResource) CreateData(name ...ObjectDataName) ObjectData {
	data := reflect.New(this.otype).Interface().(ObjectData)
	if u, ok := data.(*unstructured.Unstructured); ok {
		u.SetGroupVersionKind(this.GroupVersionKind())
	}
	if len(name) > 0 {
		data.SetName(name[0].GetName())
		data.SetNamespace(name[0].GetNamespace())
	}
	return data
}

func (this *AbstractResource) CreateListData() runtime.Object {
	return reflect.New(this.ltype).Interface().(runtime.Object)
}

func (this *AbstractResource) CheckOType(obj ObjectData, unstructured ...bool) error {
	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Ptr {
		if t.Elem() == this.otype {
			return nil
		}
		if len(unstructured) > 0 && unstructured[0] {
			if t.Elem() == unstructuredType {
				return nil
			}
		}
	}
	return errors.ErrTypeMismatch.New(obj, reflect.PtrTo(this.otype))
}
