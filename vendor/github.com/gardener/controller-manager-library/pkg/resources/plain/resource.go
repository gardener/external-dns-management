/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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
