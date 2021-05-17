/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"runtime/debug"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/gardener/controller-manager-library/pkg/resources/errors"
)

func (this *_resource) getCached(namespace, name string) (Object, error) {
	var obj ObjectData
	informer, err := this.helper.Internal.I_lookupInformer(false, namespace)
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
		return this.getCached(o.GetNamespace(), o.GetName())
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
	informer, err := this.helper.Internal.I_getInformer(false, "", nil)
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
		informer, err := this.resource.helper.Internal.I_lookupInformer(false, this.namespace)
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
