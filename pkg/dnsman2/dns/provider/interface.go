// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type DNSHandlerConfig struct {
	Log         logr.Logger
	Properties  utils.Properties
	Config      *runtime.RawExtension
	Metrics     Metrics
	RateLimiter flowcontrol.RateLimiter
}

type DNSHandlerFactory interface {
	Create(providerType string, config *DNSHandlerConfig) (DNSHandler, error)
	Supports(providerType string) bool
}

// DNSHostedZone is the interface for DNS hosted zones.
type DNSHostedZone interface {
	// ZoneID returns the unique ID of the hosted zone.
	ZoneID() dns.ZoneID
	// Key returns the provider specific key of the hosted zone.
	Key() string
	// Domain returns the domain of the hosted zone.
	Domain() string
	// MatchLevel returns the match level of the given DNS name with the hosted zone.
	MatchLevel(dnsname string) int
	// IsPrivate returns true if the hosted zone is private.
	IsPrivate() bool
}

// AreZonesEquivalent checks if the given hosted zones are equivalent (same id, key, and domain).
func AreZonesEquivalent(a, b []DNSHostedZone) bool {
	if len(a) != len(b) {
		return false
	}
outer:
	for _, i := range b {
		for _, t := range a {
			if i.ZoneID() == t.ZoneID() && i.Key() == t.Key() && i.Domain() == t.Domain() {
				continue outer
			}
		}
		return false
	}
	return true
}

type Metrics interface {
	AddGenericRequests(requestType string, n int)
	AddZoneRequests(zoneID, requestType string, n int)
}

type ChangeRequest struct {
	// TODO
}

// DNSHandler is the interface for DNS providers.
type DNSHandler interface {
	// ProviderType returns the type of the DNS provider.
	ProviderType() string
	// GetZones returns the hosted zones reachable by the DNS provider.
	GetZones(ctx context.Context) ([]DNSHostedZone, error)
	// QueryDNS queries the DNS provider for the given DNS name and record type.
	QueryDNS(ctx context.Context, zone DNSHostedZone, dnsName string, recordType dns.RecordType) ([]dns.Record, int64, error)
	// ExecuteRequests executes the given change requests in the given zone.
	ExecuteRequests(ctx context.Context, zone DNSHostedZone, reqs []*ChangeRequest) error
	// MapTargets can transform the given targets to the DNS provider special targets.
	MapTargets(dnsName string, targets []dns.Target) []dns.Target
	// Release releases the DNS provider.
	Release()
}

type DefaultDNSHandler struct {
	providerType string
}

func NewDefaultDNSHandler(providerType string) DefaultDNSHandler {
	return DefaultDNSHandler{providerType}
}

func (this *DefaultDNSHandler) ProviderType() string {
	return this.providerType
}

func (this *DefaultDNSHandler) MapTargets(_ string, targets []dns.Target) []dns.Target {
	return targets
}

type DNSHandlerCreatorFunction func(config *DNSHandlerConfig) (DNSHandler, error)
