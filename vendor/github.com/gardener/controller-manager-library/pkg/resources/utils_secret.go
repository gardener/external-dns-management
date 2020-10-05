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
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type SecretObject struct {
	Object
}

func (this *SecretObject) Secret() *api.Secret {
	return this.Data().(*api.Secret)
}

func SecretKey(namespace, name string) ObjectKey {
	return NewKey(schema.GroupKind{api.GroupName, "Secret"}, namespace, name)
}
func SecretKeyByRef(ref *api.SecretReference) ObjectKey {
	return SecretKey(ref.Namespace, ref.Name)
}

func Secret(o Object) *SecretObject {
	if o.IsA(&api.Secret{}) {
		return &SecretObject{o}
	}
	return nil
}

func GetSecret(src ResourcesSource, namespace, name string) (*SecretObject, error) {
	o, err := src.Resources().GetObjectInto(NewObjectName(namespace, name), &api.Secret{})
	if err != nil {
		return nil, err
	}

	s := Secret(o)
	if s == nil {
		return nil, errors.ErrUnexpectedType.New("access", o.Data())
	}
	return s, nil
}

func GetCachedSecret(src ResourcesSource, namespace, name string) (*SecretObject, error) {
	resource, err := src.Resources().Get(&api.Secret{})
	if err != nil {
		return nil, err
	}
	o, err := resource.GetCached(NewObjectName(namespace, name))
	if err != nil {
		return nil, err
	}

	s := Secret(o)
	if s == nil {
		return nil, errors.ErrUnexpectedType.New("access", o.Data())
	}
	return s, nil
}

func GetSecretByRef(src ResourcesSource, ref *api.SecretReference) (*SecretObject, error) {
	return GetSecret(src, ref.Namespace, ref.Name)
}

func GetCachedSecretByRef(src ResourcesSource, ref *api.SecretReference) (*SecretObject, error) {
	return GetCachedSecret(src, ref.Namespace, ref.Name)
}

func GetSecretProperties(src ResourcesSource, namespace, name string) (utils.Properties, *SecretObject, error) {
	secret, err := GetSecret(src, namespace, name)
	if err != nil {
		return nil, nil, err
	}
	props := GetSecretPropertiesFrom(secret.Secret())
	return props, secret, nil
}

func GetCachedSecretProperties(src ResourcesSource, namespace, name string) (utils.Properties, *SecretObject, error) {
	secret, err := GetCachedSecret(src, namespace, name)
	if err != nil {
		return nil, nil, err
	}
	props := GetSecretPropertiesFrom(secret.Secret())
	return props, secret, nil
}

func GetSecretPropertiesFrom(secret *api.Secret) utils.Properties {
	props := utils.Properties{}
	for k, v := range secret.Data {
		props[k] = string(v)
	}
	return props
}

func GetSecretPropertiesByRef(src ResourcesSource, ref *api.SecretReference) (utils.Properties, *SecretObject, error) {
	return GetSecretProperties(src, ref.Namespace, ref.Name)
}

func GetCachedSecretPropertiesByRef(src ResourcesSource, ref *api.SecretReference) (utils.Properties, *SecretObject, error) {
	return GetCachedSecretProperties(src, ref.Namespace, ref.Name)
}

func (this *SecretObject) GetData() map[string][]byte {
	return this.Secret().Data
}
