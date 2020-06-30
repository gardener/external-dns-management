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

package resources

import (
	"runtime/debug"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/gardener/controller-manager-library/pkg/resources/errors"
)

func (this *_resource) getCached(namespace, name string) (Object, error) {
	var obj ObjectData
	informer, err := this.helper.Internal.I_lookupInformer(namespace)
	if err != nil {
		return nil, err
	}
	if this.info.Namespaced() {
		if namespace == "" {
			return nil, errors.ErrNamespaced.New(this.GroupVersionKind())
		}
		obj, err = informer.Lister().Namespace(namespace).Get(name)
	} else {
		if namespace != "" {
			return nil, errors.ErrNotNamespaced.New(this.GroupVersionKind())
		}
		obj, err = informer.Lister().Get(name)
	}
	if err != nil {
		return nil, err
	}
	return this.helper.ObjectAsResource(obj), nil
}

func (this *_resource) GetCached(obj interface{}) (Object, error) {
	switch o := obj.(type) {
	case string:
		return this.getCached("", o)
	case ObjectData:
		if err := this.CheckOType(o); err != nil {
			return nil, err
		}
		return this.helper.ObjectAsResource(o), nil
	case ObjectKey:
		if o.GroupKind() != this.GroupKind() {
			return nil, errors.ErrResourceMismatch.New(this.GroupVersionKind(), o.GroupKind())
		}
		return this.getCached(o.Namespace(), o.Name())
	case *ObjectKey:
		if o.GroupKind() != this.GroupKind() {
			return nil, errors.ErrResourceMismatch.New(this.GroupVersionKind(), o.GroupKind())
		}
		return this.getCached(o.Namespace(), o.Name())
	case ClusterObjectKey:
		if o.GroupKind() != this.GroupKind() {
			return nil, errors.ErrResourceMismatch.New(this.GroupVersionKind(), o.GroupKind())
		}
		return this.getCached(o.Namespace(), o.Name())
	case *ClusterObjectKey:
		if o.GroupKind() != this.GroupKind() {
			return nil, errors.ErrResourceMismatch.New(this.GroupVersionKind(), o.GroupKind())
		}
		return this.getCached(o.Namespace(), o.Name())
	case ObjectName:
		return this.getCached(o.Namespace(), o.Name())
	default:
		debug.PrintStack()
		return nil, errors.ErrUnexpectedType.New("object identifier", obj)
	}
}

func (this *_resource) ListCached(selector labels.Selector) (ret []Object, err error) {
	informer, err := this.helper.Internal.I_getInformer("", nil)
	if err != nil {
		return nil, err
	}
	if selector == nil {
		selector = labels.Everything()
	}
	err = informer.Lister().List(selector, func(obj interface{}) {
		ret = append(ret, this.helper.ObjectAsResource(obj.(ObjectData)))
	})
	return ret, err
}

////////////////////////////////////////////////////////////////////////////////

func (this *namespacedResource) getLister() (NamespacedLister, error) {
	if this.lister == nil {
		informer, err := this.resource.helper.Internal.I_lookupInformer(this.namespace)
		if err != nil {
			return nil, err
		}
		this.lister = informer.Lister().Namespace(this.namespace)
	}
	return this.lister, nil
}

func (this *namespacedResource) GetCached(name string) (ret Object, err error) {

	lister, err := this.getLister()
	if err != nil {
		return nil, err
	}
	obj, err := lister.Get(name)
	if err != nil {
		return nil, err
	}
	return this.resource.helper.ObjectAsResource(obj.(ObjectData)), nil
}

func (this *namespacedResource) ListCached(selector labels.Selector) (ret []Object, err error) {
	lister, err := this.getLister()
	if err != nil {
		return nil, err
	}
	if selector == nil {
		selector = labels.Everything()
	}
	err = lister.List(selector, func(obj interface{}) {
		ret = append(ret, this.resource.helper.ObjectAsResource(obj.(ObjectData)))
	})
	return ret, err
}
