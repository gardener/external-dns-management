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

package plain

import (
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/resources/abstract"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type _resource struct {
	*abstract.AbstractResource
}

var _ Internal = &_resource{}

func newResource(
	context ResourceContext,
	otype reflect.Type,
	ltype reflect.Type,
	gvk schema.GroupVersionKind) Interface {

	return &_resource{abstract.NewAbstractResource(context, otype, ltype, gvk)}
}

/////////////////////////////////////////////////////////////////////////////////

func (this *_resource) Resources() Resources {
	return this.ResourceContext().Resources()
}

func (this *_resource) ResourceContext() ResourceContext {
	return this.AbstractResource.ResourceContext().(ResourceContext)
}

func (this *_resource) New(name ObjectName) Object {
	return this.objectAsResource(this.AbstractResource.New(name))
}

func (this *_resource) Wrap(obj ObjectData) (Object, error) {
	if err := this.CheckOType(obj); err != nil {
		return nil, err
	}
	return this.objectAsResource(obj), nil
}

func (this *_resource) objectAsResource(obj ObjectData) Object {
	return newObject(obj, this)
}
