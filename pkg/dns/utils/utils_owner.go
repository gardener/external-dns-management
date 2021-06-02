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
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var DNSOwnerType = (*api.DNSOwner)(nil)

type DNSOwnerObject struct {
	resources.Object
}

func (this *DNSOwnerObject) DNSOwner() *api.DNSOwner {
	return this.Data().(*api.DNSOwner)
}

func DNSOwner(o resources.Object) *DNSOwnerObject {
	if o.IsA(DNSOwnerType) {
		return &DNSOwnerObject{o}
	}
	return nil
}

func (this *DNSOwnerObject) Spec() *api.DNSOwnerSpec {
	return &this.DNSOwner().Spec
}

func (this *DNSOwnerObject) GetOwnerId() string {
	return this.DNSOwner().Spec.OwnerId
}

func (this *DNSOwnerObject) IsEnabled() bool {
	a := this.DNSOwner().Spec.Active
	return a == nil || *a
}

func (this *DNSOwnerObject) IsActive() bool {
	if this.IsEnabled() {
		valid := this.DNSOwner().Spec.ValidUntil
		return valid == nil || valid.After(time.Now())
	}
	return false
}

func (this *DNSOwnerObject) ValidUntil() *metav1.Time {
	return this.DNSOwner().Spec.ValidUntil
}

func (this *DNSOwnerObject) GetCounts() map[string]int {
	return this.DNSOwner().Status.Entries.ByType
}

func (this *DNSOwnerObject) GetCount() int {
	return this.DNSOwner().Status.Entries.Amount
}
