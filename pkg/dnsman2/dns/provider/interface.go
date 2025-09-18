// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// DNSHandlerFactory is the interface for DNS handler factories.
type DNSHandlerFactory interface {
	// Create creates a DNSHandler for the given provider type and config.
	Create(providerType string, config *DNSHandlerConfig) (DNSHandler, error)
	// GetAdapter returns a DNSHandlerAdapter for the given provider type code.
	GetDNSHandlerAdapter(typecode string) (DNSHandlerAdapter, error)
	// Supports returns true if the factory supports the given provider type.
	Supports(providerType string) bool
	// GetTargetsMapper returns a TargetsMapper for the given provider type.
	GetTargetsMapper(providerType string) (TargetsMapper, error)
}

// DNSHostedZone is the interface for DNS hosted zones.
type DNSHostedZone interface {
	// ZoneID returns the unique ID of the hosted zone.
	ZoneID() dns.ZoneID
	// Key returns the provider specific key of the hosted zone.
	Key() string
	// Domain returns the domain of the hosted zone.
	Domain() string
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
	MetricsRequestTypeListZones MetricsRequestType = "list_zones"
	// MetricsRequestTypeListZonesPages is the metrics used for paginated listing of DNS zones.
	MetricsRequestTypeListZonesPages MetricsRequestType = "list_zones_pages"

	// MetricsRequestTypeListRecords is the metrics used for listing DNS records.
	MetricsRequestTypeListRecords MetricsRequestType = "list_records"
	// MetricsRequestTypeListRecordPages is the metrics used for paginated listing of DNS records.
	MetricsRequestTypeListRecordPages MetricsRequestType = "list_records_pages"

	// MetricsRequestTypeUpdateRecords is the metrics used for updating DNS records.
	MetricsRequestTypeUpdateRecords MetricsRequestType = "update_records"
	// MetricsRequestTypeUpdateRecordPages is the metrics used for paginated updating of DNS records.
	MetricsRequestTypeUpdateRecordPages MetricsRequestType = "update_records_pages"

	// MetricsRequestTypeCreateRecords is the metrics used for creating DNS records.
	MetricsRequestTypeCreateRecords MetricsRequestType = "create_records"
	// MetricsRequestTypeDeleteRecords is the metrics used for deleting DNS records.
	MetricsRequestTypeDeleteRecords MetricsRequestType = "delete_records"

	// MetricsRequestTypeCachedGetZones is the metrics used for cached retrieval of DNS zones.
	MetricsRequestTypeCachedGetZones MetricsRequestType = "cached_getzones"
)

// Metrics is the interface for reporting DNS provider metrics.
type Metrics interface {
	// AddGenericRequests adds generic request metrics.
	AddGenericRequests(requestType MetricsRequestType, n int)
	// AddZoneRequests adds zone-specific request metrics.
	AddZoneRequests(zoneID string, requestType MetricsRequestType, n int)
}

// ChangeRequests holds a set of DNS record change requests for a DNS name.
type ChangeRequests struct {
	Name    dns.DNSSetName
	Updates map[dns.RecordType]*ChangeRequestUpdate
}

// NewChangeRequests creates a new ChangeRequests for the given DNS name.
func NewChangeRequests(name dns.DNSSetName) *ChangeRequests {
	return &ChangeRequests{
		Name:    name,
		Updates: make(map[dns.RecordType]*ChangeRequestUpdate),
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
	// GetCustomQueryDNSFunc returns a custom query function if required as the authoritative DNS server is not reachable.
	GetCustomQueryDNSFunc(zoneInfo dns.ZoneInfo, factory utils.QueryDNSFactoryFunc) (CustomQueryDNSFunc, error)
	// ExecuteRequests executes the given change requests in the given zone.
	ExecuteRequests(ctx context.Context, zone DNSHostedZone, requests ChangeRequests) error
	// Release releases the DNS provider.
	Release()
}

// DNSHandlerAdapter is an interface for input validation of provider secrets and configuration.
type DNSHandlerAdapter interface {
	// ProviderType returns the type of the DNS provider.
	ProviderType() string
	// ValidateCredentialsAndProviderConfig validates the provider credentials and configuration.
	ValidateCredentialsAndProviderConfig(properties utils.Properties, config *runtime.RawExtension) error
}

// TargetsMapper is an interface for mapping DNS targets to provider-specific targets.
type TargetsMapper interface {
	// MapTargets can transform the given targets to the DNS provider special targets.
	MapTargets(targets []dns.Target) []dns.Target
}

// CustomQueryDNSFunc is a function type for custom DNS queries.
type CustomQueryDNSFunc func(ctx context.Context, zoneInfo dns.ZoneInfo, dnsName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error)

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

// DNSHandlerCreatorFunction is a function type for creating DNSHandler instances.
type DNSHandlerCreatorFunction func(config *DNSHandlerConfig) (DNSHandler, error)
