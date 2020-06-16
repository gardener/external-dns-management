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
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns/provider/errors"
)

var DNSProviderType = (*api.DNSProvider)(nil)

type DNSProviderObject struct {
	resources.Object
}

func (this *DNSProviderObject) DNSProvider() *api.DNSProvider {
	return this.Data().(*api.DNSProvider)
}

func DNSProviderKey(namespace, name string) resources.ObjectKey {
	return resources.NewKey(schema.GroupKind{Group: api.GroupName, Kind: api.DNSProviderKind}, namespace, name)
}

func (this *DNSProviderObject) Spec() *api.DNSProviderSpec {
	return &this.DNSProvider().Spec
}
func (this *DNSProviderObject) Status() interface{} {
	return this.DNSProviderStatus()
}

func (this *DNSProviderObject) DNSProviderStatus() *api.DNSProviderStatus {
	return &this.DNSProvider().Status
}

func (this *DNSProviderObject) TypeCode() string {
	return this.DNSProvider().Spec.Type
}

func (this *DNSProviderObject) SetStateWithError(state string, err error) bool {
	type causer interface {
		error
		Cause() error
	}

	message := err.Error()
	handlerErrorMsg := ""
	for {
		if cerr, ok := err.(causer); ok {
			cause := cerr.Cause()
			if cause == nil || cause == err {
				break
			}
			if errors.IsHandlerError(cause) {
				handlerErrorMsg = cause.Error()
				break
			}
			err = cause
		} else {
			break
		}
	}
	if handlerErrorMsg != "" {
		prefix := message
		if len(message) > len(handlerErrorMsg) && strings.HasSuffix(message, handlerErrorMsg) {
			prefix = message[:len(message)-len(handlerErrorMsg)]
		}
		return this.SetState(api.STATE_ERROR, message, prefix)
	}
	return this.SetState(api.STATE_ERROR, message)
}

func (this *DNSProviderObject) SetState(state, message string, commonMessagePrefix ...string) bool {
	mod := &utils.ModificationState{}
	status := &this.DNSProvider().Status
	if len(commonMessagePrefix) != 1 || status.Message == nil || !strings.HasPrefix(*status.Message, commonMessagePrefix[0]) {
		// only update if prefix has changed. This avoids race conditions (state update <-> reconcilie) if message
		// contains time stamps or correlation ids
		mod.AssureStringPtrValue(&status.Message, message)
	}
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
