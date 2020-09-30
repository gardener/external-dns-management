/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	api "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources/errors"
)

type IngressObject struct {
	Object
}

func (this *IngressObject) Ingress() *api.Ingress {
	return this.Data().(*api.Ingress)
}

func IngressKey(namespace, name string) ObjectKey {
	return NewKey(schema.GroupKind{api.GroupName, "Ingress"}, namespace, name)
}

func Ingress(o Object) *IngressObject {
	if o.IsA(&api.Ingress{}) {
		return &IngressObject{o}
	}
	return nil
}

func GetIngress(src ResourcesSource, namespace, name string) (*IngressObject, error) {
	resources := src.Resources()
	o, err := resources.GetObjectInto(NewObjectName(namespace, name), &api.Ingress{})
	if err != nil {
		return nil, err
	}

	s := Ingress(o)
	if s == nil {
		return nil, errors.ErrUnexpectedType.New("ingress", o.Data())
	}
	return s, nil
}

func GetCachedIngress(src ResourcesSource, namespace, name string) (*IngressObject, error) {
	resource, err := src.Resources().Get(&api.Ingress{})
	if err != nil {
		return nil, err
	}
	o, err := resource.GetCached(NewObjectName(namespace, name))
	if err != nil {
		return nil, err
	}

	s := Ingress(o)
	if s == nil {
		return nil, errors.ErrUnexpectedType.New("ingress", o.Data())
	}
	return s, nil
}
