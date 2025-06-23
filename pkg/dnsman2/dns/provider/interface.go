// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// DNSHandlerConfig holds configuration for creating a DNSHandler.
type DNSHandlerConfig struct {
	Log         logr.Logger
	Properties  utils.Properties
	Config      *runtime.RawExtension
	Metrics     Metrics
	RateLimiter flowcontrol.RateLimiter
}

// DNSHandlerFactory is the interface for DNS handler factories.
type DNSHandlerFactory interface {
	// Create creates a DNSHandler for the given provider type and config.
	Create(providerType string, config *DNSHandlerConfig) (DNSHandler, error)
	// Supports returns true if the factory supports the given provider type.
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

// MetricsRequestType is a string type for metrics request types.
type MetricsRequestType string

// Metrics request type constants for various DNS provider operations.
const (
	// MetricsRequestTypeListZones is the metrics used for listing DNS zones.
	MetricsRequestTypeListZones = "list_zones"
	// MetricsRequestTypeListZonesPages is the metrics used for paginated listing of DNS zones.
	MetricsRequestTypeListZonesPages = "list_zones_pages"

	// MetricsRequestTypeListRecords is the metrics used for listing DNS records.
	MetricsRequestTypeListRecords = "list_records"
	// MetricsRequestTypeListRecordPages is the metrics used for paginated listing of DNS records.
	MetricsRequestTypeListRecordPages = "list_records_pages"

	// MetricsRequestTypeUpdateRecords is the metrics used for updating DNS records.
	MetricsRequestTypeUpdateRecords = "update_records"
	// MetricsRequestTypeUpdateRecordPages is the metrics used for paginated updating of DNS records.
	MetricsRequestTypeUpdateRecordPages = "update_records_pages"

	// MetricsRequestTypeCreateRecords is the metrics used for creating DNS records.
	MetricsRequestTypeCreateRecords = "create_records"
	// MetricsRequestTypeDeleteRecords is the metrics used for deleting DNS records.
	MetricsRequestTypeDeleteRecords = "delete_records"

	// MetricsRequestTypeCachedGetZones is the metrics used for cached retrieval of DNS zones.
	MetricsRequestTypeCachedGetZones = "cached_getzones"
)

// Metrics is the interface for reporting DNS provider metrics.
type Metrics interface {
	// AddGenericRequests adds generic request metrics.
	AddGenericRequests(requestType string, n int)
	// AddZoneRequests adds zone-specific request metrics.
	AddZoneRequests(zoneID, requestType string, n int)
}

// DoneHandler is the interface for handling completion of DNS change requests.
type DoneHandler interface {
	// SetInvalid marks the request as invalid.
	SetInvalid(err error)
	// Failed marks the request as failed.
	Failed(err error)
	// Throttled marks the request as throttled.
	Throttled()
	// Succeeded marks the request as succeeded.
	Succeeded()
}

// ChangeRequests holds a set of DNS record change requests for a DNS name.
type ChangeRequests struct {
	Name    dns.DNSSetName
	Updates map[dns.RecordType]*ChangeRequestUpdate
	Done    DoneHandler
}

// NewChangeRequests creates a new ChangeRequests for the given DNS name and DoneHandler.
func NewChangeRequests(name dns.DNSSetName, done DoneHandler) *ChangeRequests {
	return &ChangeRequests{
		Name:    name,
		Updates: make(map[dns.RecordType]*ChangeRequestUpdate),
		Done:    done,
	}
}

// String returns a string representation of the ChangeRequests.
func (cr *ChangeRequests) String() string {
	return fmt.Sprintf("ChangeRequests(Name: %s, Updates: %v)", cr.Name, cr.Updates)
}

// ChangeRequestUpdate holds the old and new DNS record sets for a change.
type ChangeRequestUpdate struct {
	Old *dns.RecordSet
	New *dns.RecordSet
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
	ExecuteRequests(ctx context.Context, zone DNSHostedZone, requests ChangeRequests) error
	// MapTargets can transform the given targets to the DNS provider special targets.
	MapTargets(dnsName string, targets []dns.Target) []dns.Target
	// Release releases the DNS provider.
	Release()
}

// DefaultDNSHandler is a default implementation of DNSHandler for a provider type.
type DefaultDNSHandler struct {
	providerType string
}

// NewDefaultDNSHandler creates a new DefaultDNSHandler for the given provider type.
func NewDefaultDNSHandler(providerType string) DefaultDNSHandler {
	return DefaultDNSHandler{providerType}
}

// ProviderType returns the provider type.
func (this *DefaultDNSHandler) ProviderType() string {
	return this.providerType
}

// MapTargets returns the given targets unchanged.
func (this *DefaultDNSHandler) MapTargets(_ string, targets []dns.Target) []dns.Target {
	return targets
}

// DNSHandlerCreatorFunction is a function type for creating DNSHandler instances.
type DNSHandlerCreatorFunction func(config *DNSHandlerConfig) (DNSHandler, error)
