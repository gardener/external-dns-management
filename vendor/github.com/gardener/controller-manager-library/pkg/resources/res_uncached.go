/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"reflect"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/controller-manager-library/pkg/resources/errors"
)

func (this *AbstractResource) Create(obj ObjectData) (Object, error) {
	if o, ok := obj.(Object); ok {
		obj = o.Data()
	}
	if err := this.CheckOType(obj); err != nil {
		return nil, err
	}
	result, err := this.helper.Internal.I_create(obj)
	if err != nil {
		return nil, err
	}
	return this.helper.ObjectAsResource(result), nil
}

func (this *AbstractResource) CreateOrUpdate(obj ObjectData) (Object, error) {
	if o, ok := obj.(Object); ok {
		obj = o.Data()
	}
	if err := this.CheckOType(obj); err != nil {
		return nil, err
	}
	if obj.GetResourceVersion() == "" {
		result, err := this.helper.Internal.I_create(obj)
		if err == nil {
			return this.helper.ObjectAsResource(result), err

		}
		if !k8serr.IsAlreadyExists(err) {
			return nil, err
		}
		result.SetName(obj.GetName())
		result.SetNamespace(obj.GetNamespace())
		err = this.helper.Internal.I_get(result)
		if err != nil {
			return nil, err
		}
		obj.SetResourceVersion(result.GetResourceVersion())
	}
	result, err := this.helper.Internal.I_update(obj)
	if err != nil {
		return nil, err
	}

	return this.helper.ObjectAsResource(result), nil
}

func (this *AbstractResource) Update(obj ObjectData) (Object, error) {
	if o, ok := obj.(Object); ok {
		obj = o.Data()
	}
	if err := this.CheckOType(obj); err != nil {
		return nil, err
	}
	result, err := this.helper.Internal.I_update(obj)
	if err != nil {
		return nil, err
	}
	return this.helper.ObjectAsResource(result), nil
}

func (this *AbstractResource) Modify(obj ObjectData, modifier Modifier) (ObjectData, bool, error) {
	if o, ok := obj.(Object); ok {
		obj = o.Data()
	}
	if err := this.CheckOType(obj); err != nil {
		return nil, false, err
	}
	return this.helper.Internal.I_modify(obj, false, false, false, modifier)
}

func (this *AbstractResource) ModifyByName(obj ObjectDataName, modifier Modifier) (Object, bool, error) {
	return this.helper.Internal.I_modifyByName(obj, false, false, modifier)
}

func (this *AbstractResource) CreateOrModifyByName(obj ObjectDataName, modifier Modifier) (Object, bool, error) {
	return this.helper.Internal.I_modifyByName(obj, false, true, modifier)
}

func (this *AbstractResource) ModifyStatus(obj ObjectData, modifier Modifier) (ObjectData, bool, error) {
	if o, ok := obj.(Object); ok {
		obj = o.Data()
	}
	if err := this.CheckOType(obj); err != nil {
		return nil, false, err
	}
	return this.helper.Internal.I_modify(obj, true, false, false, modifier)
}

func (this *AbstractResource) ModifyStatusByName(obj ObjectDataName, modifier Modifier) (Object, bool, error) {
	return this.helper.Internal.I_modifyByName(obj, true, false, modifier)
}

func (this *AbstractResource) Delete(obj ObjectData) error {
	if o, ok := obj.(Object); ok {
		obj = o.Data()
	}
	if err := this.CheckOType(obj); err != nil {
		return err
	}
	err := this.helper.Internal.I_delete(obj)
	if err != nil {
		return err
	}
	return nil
}

func (this *AbstractResource) DeleteByName(obj ObjectDataName) error {
	return this.helper.Internal.I_delete(obj)
}

func (this *AbstractResource) handleList(result runtime.Object) (ret []Object, err error) {
	v := reflect.ValueOf(result)
	iv := v.Elem().FieldByName("Items")
	if iv.Kind() != reflect.Slice {
		return nil, errors.NewInvalid("invalid Items field for %T", result)
	}
	for i := 0; i < iv.Len(); i++ {
		ret = append(ret, this.helper.ObjectAsResource(iv.Index(i).Addr().Interface().(ObjectData)))
	}
	return ret, nil
}

func (this *AbstractResource) GetInto(name ObjectName, obj ObjectData) (Object, error) {
	if o, ok := obj.(Object); ok {
		obj = o.Data()
	}
	if err := this.CheckOType(obj, true); err != nil {
		return nil, err
	}
	return this.helper.Get(name.Namespace(), name.Name(), obj)
}

func (this *AbstractResource) GetInto1(obj ObjectData) (Object, error) {
	if o, ok := obj.(Object); ok {
		obj = o.Data()
	}
	if err := this.CheckOType(obj, true); err != nil {
		return nil, err
	}
	return this.helper.Get(obj.GetNamespace(), obj.GetName(), obj)
}

func (this *AbstractResource) Get_(obj interface{}) (Object, error) {
	return this.Get(obj)
}
func (this *AbstractResource) Get(obj interface{}) (Object, error) {
	gvk := this.GroupVersionKind()
	switch o := obj.(type) {
	case string:
		if this.Namespaced() {
			return nil, errors.ErrNamespaced.New(gvk)
		}
		return this.helper.Get("", o, nil)
	case ObjectData:
		if err := this.CheckOType(o); err != nil {
			return nil, err
		}
		return this.helper.Get(o.GetNamespace(), o.GetName(), nil)
	case ObjectKey:
		if o.GroupKind() != this.GroupKind() {
			return nil, errors.ErrResourceMismatch.New(gvk, o.GroupKind())
		}
		return this.helper.Get(o.Namespace(), o.Name(), nil)
	case *ObjectKey:
		if o.GroupKind() != this.GroupKind() {
			return nil, errors.ErrResourceMismatch.New(gvk, o.GroupKind())
		}
		return this.helper.Get(o.Namespace(), o.Name(), nil)
	case ClusterObjectKey:
		if o.GroupKind() != this.GroupKind() {
			return nil, errors.ErrResourceMismatch.New(gvk, o.GroupKind())
		}
		return this.helper.Get(o.Namespace(), o.Name(), nil)
	case *ClusterObjectKey:
		if o.GroupKind() != this.GroupKind() {
			return nil, errors.ErrResourceMismatch.New(gvk, o.GroupKind())
		}
		return this.helper.Get(o.Namespace(), o.Name(), nil)
	case ObjectName:
		return this.helper.Get(o.Namespace(), o.Name(), nil)
	default:
		return nil, errors.ErrUnexpectedType.New("object identity", obj)
	}
}

func (this *AbstractResource) List(opts metav1.ListOptions) (ret []Object, err error) {
	return this.helper.Internal.I_list(metav1.NamespaceAll, opts)
}

////////////////////////////////////////////////////////////////////////////////

func (this *namespacedResource) GetInto(name string, obj ObjectData) (ret Object, err error) {
	if o, ok := obj.(Object); ok {
		obj = o.Data()
	}
	if err := this.resource.CheckOType(obj); err != nil {
		return nil, err
	}
	return this.resource.helper.Get(this.namespace, name, obj)
}

func (this *namespacedResource) Get(name string) (ret Object, err error) {
	return this.resource.helper.Get(this.namespace, name, nil)
}

func (this *namespacedResource) List(opts metav1.ListOptions) (ret []Object, err error) {
	if !this.resource.Namespaced() {
		return nil, errors.ErrNotNamespaced.New(this.resource.GroupVersionKind())
	}
	return this.resource.helper.Internal.I_list(this.namespace, opts)
}
