/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources/errors"
)

type ServiceObject struct {
	Object
}

func (this *ServiceObject) Service() *api.Service {
	return this.Data().(*api.Service)
}

func ServiceKey(namespace, name string) ObjectKey {
	return NewKey(schema.GroupKind{api.GroupName, "Service"}, namespace, name)
}

func Service(o Object) *ServiceObject {
	if o.IsA(&api.Service{}) {
		return &ServiceObject{o}
	}
	return nil
}

func (this *ServiceObject) Status() *api.ServiceStatus {
	return &this.Service().Status
}

func (this *ServiceObject) Spec() *api.ServiceSpec {
	return &this.Service().Spec
}

func GetService(src ResourcesSource, namespace, name string) (*ServiceObject, error) {
	resources := src.Resources()
	o, err := resources.GetObjectInto(NewObjectName(namespace, name), &api.Service{})
	if err != nil {
		return nil, err
	}

	s := Service(o)
	if s == nil {
		return nil, errors.ErrUnexpectedType.New("service", o.Data())
	}
	return s, nil
}

func GetCachedService(src ResourcesSource, namespace, name string) (*ServiceObject, error) {
	resource, err := src.Resources().Get(&api.Service{})
	if err != nil {
		return nil, err
	}
	o, err := resource.GetCached(NewObjectName(namespace, name))
	if err != nil {
		return nil, err
	}

	s := Service(o)
	if s == nil {
		return nil, errors.ErrUnexpectedType.New("service", o.Data())
	}
	return s, nil
}
