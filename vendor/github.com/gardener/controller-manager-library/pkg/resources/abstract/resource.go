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
