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
	"github.com/gardener/controller-manager-library/pkg/resources"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var DNSEntryType = (*api.DNSEntry)(nil)

type DNSEntryObject struct {
	resources.Object
}

func (this *DNSEntryObject) DNSEntry() *api.DNSEntry {
	return this.Data().(*api.DNSEntry)
}

func DNSEntry(o resources.Object) *DNSEntryObject {
	if o.IsA(DNSEntryType) {
		return &DNSEntryObject{o}
	}
	return nil
}

func (this *DNSEntryObject) Spec() *api.DNSEntrySpec {
	return &this.DNSEntry().Spec
}
func (this *DNSEntryObject) Status() *api.DNSEntryStatus {
	return &this.DNSEntry().Status
}

func (this *DNSEntryObject) GetDNSName() string {
	return this.DNSEntry().Spec.DNSName
}
func (this *DNSEntryObject) GetTargets() []string {
	return this.DNSEntry().Spec.Targets
}
func (this *DNSEntryObject) GetOwnerId() *string {
	return this.DNSEntry().Spec.OwnerId
}
func (this *DNSEntryObject) GetTTL() *int64 {
	return this.DNSEntry().Spec.TTL
}
func (this *DNSEntryObject) GetCNameLookupInterval() *int64 {
	return this.DNSEntry().Spec.CNameLookupInterval
}
