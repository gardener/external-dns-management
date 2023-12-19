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
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
)

type TargetProvider interface {
	Targets() Targets
	TTL() int64
	OwnerId() string
	RoutingPolicy() *dns.RoutingPolicy
}

type DNSSpecification interface {
	resources.Object
	GetDNSName() string
	GetSetIdentifier() string
	GetTTL() *int64
	GetOwnerId() *string
	GetTargets() []string
	GetText() []string
	GetCNameLookupInterval() *int64
	GetReference() *api.EntryReference
	BaseStatus() *api.DNSBaseStatus
	GetRoutingPolicy() *dns.RoutingPolicy

	GetTargetSpec(TargetProvider) TargetSpec

	RefreshTime() time.Time
	ValidateSpecial() error
	AcknowledgeTargets(targets []string) bool
	AcknowledgeRoutingPolicy(policy *dns.RoutingPolicy) bool
}

func DNSObject(data resources.Object, _ ...any) DNSSpecification {
	switch data.Data().(type) {
	case *api.DNSEntry:
		return DNSEntry(data)
	case *api.DNSLock:
		return DNSLock(data)
	default:
		return nil
	}
}
