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

package utils

import (
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"github.com/gardener/controller-manager-library/pkg/resources"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var DNSProviderType = (*api.DNSProvider)(nil)

type DNSProviderObject struct {
	resources.Object
}

func (this *DNSProviderObject) DNSProvider() *api.DNSProvider {
	return this.Data().(*api.DNSProvider)
}

func DNSProviderKey(namespace, name string) resources.ObjectKey {
	return resources.NewKey(schema.GroupKind{api.GroupName, api.DNSProviderKind}, namespace, name)
}

func (this *DNSProviderObject) Spec() *api.DNSProviderSpec {
	return &this.DNSProvider().Spec
}
func (this *DNSProviderObject) Status() *api.DNSProviderStatus {
	return &this.DNSProvider().Status
}

func (this *DNSProviderObject) TypeCode() string {
	return this.DNSProvider().Spec.Type
}

func (this *DNSProviderObject) SetState(state, message string) bool {
	mod := &utils.ModificationState{}
	status := &this.DNSProvider().Status
	mod.AssureStringPtrValue(&status.Message, message)
	mod.AssureStringValue(&status.State, state)
	mod.AssureInt64Value(&status.ObservedGeneration, this.DNSProvider().Generation)
	return mod.IsModified()
}

func (this *DNSProviderObject) SetSelection(included, excluded utils.StringSet, target *api.DNSSelectionStatus) bool {
	modified := false
	old_inc := utils.NewStringSetByArray(target.Included)
	if !old_inc.Equals(included) {
		modified = true
		target.Included = included.AsArray()
	}
	old_exc := utils.NewStringSetByArray(target.Excluded)
	if !old_exc.Equals(excluded) {
		modified = true
		target.Excluded = excluded.AsArray()
	}
	return modified
}

func DNSProvider(o resources.Object) *DNSProviderObject {
	if o.IsA(DNSProviderType) {
		return &DNSProviderObject{o}
	}
	return nil
}

func GetDNSProvider(src resources.ResourcesSource, namespace, name string) (*DNSProviderObject, error) {
	resources := src.Resources()
	o, err := resources.GetObject(DNSProviderKey(namespace, name))
	if err != nil {
		return nil, err
	}

	s := DNSProvider(o)

	if s == nil {
		return nil, fmt.Errorf("oops, unexpected type for DNSProvider: %T", o.Data())
	}
	return s, nil
}
