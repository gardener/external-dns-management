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

	"github.com/gardener/controller-manager-library/pkg/resources"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
)

var _ DNSSpecification = (*DNSLockObject)(nil)

var DNSLockType = (*api.DNSEntry)(nil)

type DNSLockObject struct {
	resources.Object
}

func (this *DNSLockObject) DNSLock() *api.DNSLock {
	return this.Data().(*api.DNSLock)
}

func DNSLock(o resources.Object) *DNSLockObject {
	if o.IsA(DNSLockType) {
		return &DNSLockObject{o}
	}
	return nil
}

func (this *DNSLockObject) Spec() *api.DNSLockSpec {
	return &this.DNSLock().Spec
}

func (this *DNSLockObject) StatusField() interface{} {
	return this.Status()
}

func (this *DNSLockObject) Status() *api.DNSBaseStatus {
	return &this.DNSLock().Status
}

func (this *DNSLockObject) BaseStatus() *api.DNSBaseStatus {
	return &this.DNSLock().Status
}

func (this *DNSLockObject) GetDNSName() string {
	return this.DNSLock().Spec.DNSName
}
func (this *DNSLockObject) GetTargets() []string {
	return nil
}
func (this *DNSLockObject) GetText() []string {
	attrs := []string{}
	attrs = append(attrs, fmt.Sprintf("%s=%d", dns.ATTR_TIMESTAMP, this.Spec().Timestamp.Unix()))
	if this.Spec().Attributes != nil {
		for k, v := range this.Spec().Attributes {
			attrs = append(attrs, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return attrs
}
func (this *DNSLockObject) GetOwnerId() *string {
	return &this.DNSLock().Spec.LockId
}
func (this *DNSLockObject) GetTTL() *int64 {
	return this.DNSLock().Spec.TTL
}
func (this *DNSLockObject) GetCNameLookupInterval() *int64 {
	return nil
}
func (this *DNSLockObject) GetReference() *api.EntryReference {
	return nil
}

func (this *DNSLockObject) ValidateSpecial() error {
	if len(this.Spec().Attributes) == 0 {
		return fmt.Errorf("no attributes defined")
	}
	return nil
}

func (this *DNSLockObject) AcknowledgeTargets(targets []string) bool {
	return false
}
