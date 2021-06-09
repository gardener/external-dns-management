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

type DNSSpecification interface {
	resources.Object
	GetDNSName() string
	GetTTL() *int64
	GetOwnerId() *string
	GetTargets() []string
	GetText() []string
	GetCNameLookupInterval() *int64
	GetReference() *api.EntryReference
	BaseStatus() *api.DNSBaseStatus

	ValidateSpecial() error
	AcknowledgeTargets(targets []string) bool
}

var _ DNSSpecification = (*DNSEntryObject)(nil)

func DNSObject(data resources.Object) DNSSpecification {
	switch data.Data().(type) {
	case *api.DNSEntry:
		return DNSEntry(data)
	case *api.DNSLock:
		return DNSLock(data)
	default:
		return nil
	}
}
