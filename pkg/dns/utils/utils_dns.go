// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
