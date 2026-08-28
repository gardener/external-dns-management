// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package gdc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	globalnetworkingv1 "github.com/googlecloudplatform/google-distributed-cloud-apis/pkg/apis/public/global/networking/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gdcclient "github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client"
	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/auth"
	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/constants"
	dnsconst "github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var sharedScheme = runtime.NewScheme()

func init() {
	_ = globalnetworkingv1.AddToScheme(sharedScheme)
	_ = corev1.AddToScheme(sharedScheme)
}

// handler implements the provider.DNSHandler interface for GDC (gdch-dns).
// It orchestrates direct synchronization between Gardener external-dns-management
// and GDC ManagedDNSZones and ResourceRecordSets custom resources.
type handler struct {
	project string
	client  client.Client
	config  provider.DNSHandlerConfig
}

var _ provider.DNSHandler = &handler{}

// ProviderType returns the unique type identifier of this DNS provider.
func (h *handler) ProviderType() string {
	return ProviderType
}

// Release performs any necessary cleanup or connection teardowns when the handler is closed.
func (h *handler) Release() {}

// GetZones discovers and lists all ManagedDNSZone Custom Resources in the GDC project namespace.
// It maps them to Gardener's internal provider.DNSHostedZone structure for mapping and ownership checks.
func (h *handler) GetZones(ctx context.Context) ([]provider.DNSHostedZone, error) {
	// Apply rate limiting and report metrics to prevent API throttling and enable operational visibility.
	h.config.RateLimiter.Accept()
	h.config.Metrics.AddGenericRequests(provider.MetricsRequestTypeListZones, 1)

	managedZoneList := &globalnetworkingv1.ManagedDNSZoneList{}
	listOptions := client.InNamespace(h.project)
	if err := h.client.List(ctx, managedZoneList, listOptions); err != nil {
		return nil, fmt.Errorf("failed to list ManagedDNSZones in project %q: %w", h.project, err)
	}

	var zones []provider.DNSHostedZone
	for _, zone := range managedZoneList.Items {
		hostedZone := provider.NewDNSHostedZone(
			h.ProviderType(),
			h.makeZoneID(zone.Name),
			dns.NormalizeDomainName(zone.Spec.DNSName),
			zone.Name,
			false,
		)
		zones = append(zones, hostedZone)
	}

	h.config.Log.Info("Discovered GDC DNS zones", "count", len(zones), "provider", h.ProviderType(), "project", h.project)
	return zones, nil
}

// GetCustomQueryDNSFunc registers and returns the custom authoritative resolver query function
// for a given DNS zone. This redirects Gardener's record lookups to GDC API servers directly.
func (h *handler) GetCustomQueryDNSFunc(_ dns.ZoneInfo, _ utils.QueryDNSFactoryFunc) (provider.CustomQueryDNSFunc, error) {
	return func(ctx context.Context, localZoneInfo dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
		return h.queryDNS(ctx, localZoneInfo, setName, recordType)
	}, nil
}

// queryDNS queries ResourceRecordSets custom resources directly from GDC APIs.
func (h *handler) queryDNS(ctx context.Context, _ dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
	// Apply rate limiting and report metrics to prevent API throttling and enable operational visibility.
	h.config.RateLimiter.Accept()
	h.config.Metrics.AddGenericRequests(provider.MetricsRequestTypeListRecords, 1)

	// Calculate unique ResourceRecordSet name according to GDC controller schemes.
	recordSetName := dnsconst.GetDNSRecordSetName(string(recordType), setName.DNSName)
	gdcRecordSet := &globalnetworkingv1.ResourceRecordSet{}

	err := h.client.Get(ctx, client.ObjectKey{
		Name:      recordSetName,
		Namespace: h.project,
	}, gdcRecordSet)

	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ResourceRecordSet %q: %w", recordSetName, err)
	}

	ttl := int64(0)
	if gdcRecordSet.Spec.TTLSeconds != nil {
		ttl = int64(*gdcRecordSet.Spec.TTLSeconds)
	}

	records := make([]*dns.Record, 0, len(gdcRecordSet.Spec.RRData))
	for _, rrdata := range gdcRecordSet.Spec.RRData {
		val := rrdata
		if recordType == dns.TypeTXT {
			if u, err := strconv.Unquote(rrdata); err == nil {
				val = u
			}
		}
		records = append(records, &dns.Record{Value: val})
	}

	h.config.Log.Info("Resolved DNS record from GDC API", "name", setName.DNSName, "type", recordType, "records", len(records), "provider", h.ProviderType(), "project", h.project)
	return dns.NewRecordSet(recordType, ttl, records), nil
}

// ExecuteRequests executes DNS record changes by creating, updating, or deleting ResourceRecordSet
// Custom Resources in the GDC project namespace.
// It processes a batch of updates mapped by record type:
//   - If the update's New set is nil, the record is scheduled for deletion, and we delete the GDC ResourceRecordSet.
//   - Otherwise, the record is scheduled for creation or update. We resolve the records values, construct the spec,
//     and perform a CreateOrUpdate operation to reconcile the desired state inside the GDC API server.
func (h *handler) ExecuteRequests(ctx context.Context, zone provider.DNSHostedZone, requests provider.ChangeRequests) error {
	var succeeded, failed int
	for recordType, update := range requests.Updates {
		h.config.RateLimiter.Accept()
		h.config.Metrics.AddZoneRequests(zone.ZoneID().ID, provider.MetricsRequestTypeUpdateRecords, 1)

		recordSetName := dnsconst.GetDNSRecordSetName(string(recordType), requests.Name.DNSName)
		gdcRecordSet := &globalnetworkingv1.ResourceRecordSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      recordSetName,
				Namespace: h.project,
			},
		}

		if update.New == nil {
			if err := h.client.Delete(ctx, gdcRecordSet); client.IgnoreNotFound(err) != nil {
				failed++
				h.config.Log.Error(err, "Failed to delete ResourceRecordSet", "name", recordSetName)
				continue
			}
			succeeded++
			h.config.Log.Info("Deleted GDC ResourceRecordSet successfully", "name", recordSetName, "provider", h.ProviderType(), "project", h.project)
		} else {
			ttl := uint32(update.New.TTL) // #nosec G115 -- TTL is safe bounded integer
			rrdata := make([]string, 0, len(update.New.Records))
			for _, r := range update.New.Records {
				val := r.Value
				if recordType == dns.TypeTXT {
					val = ensureQuotedText(val)
				}
				rrdata = append(rrdata, val)
			}

			_, err := controllerutil.CreateOrUpdate(ctx, h.client, gdcRecordSet, func() error {
				gdcRecordSet.Spec = globalnetworkingv1.ResourceRecordSetSpec{
					Name:       trimTrailingDot(requests.Name.DNSName),
					Type:       string(recordType),
					TTLSeconds: &ttl,
					RRData:     rrdata,
					DNSZone:    zone.Key(),
				}
				return nil
			})
			if err != nil {
				failed++
				h.config.Log.Error(err, "Failed to apply ResourceRecordSet", "name", recordSetName)
				continue
			}
			succeeded++
			h.config.Log.Info("Applied GDC ResourceRecordSet successfully", "name", recordSetName, "provider", h.ProviderType(), "project", h.project)
		}
	}

	if failed > 0 {
		return fmt.Errorf("failed to execute %d of %d record change updates", failed, succeeded+failed)
	}
	return nil
}

// makeZoneID formats a GDC zone ID utilizing the project and resource name.
func (h *handler) makeZoneID(zoneName string) string {
	return fmt.Sprintf("%s/%s", h.project, zoneName)
}

// NewHandler instantiates a new GDC DNSHandler using the provided configuration.
func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	h := &handler{
		config: *config,
	}

	gdchConfigStr := h.config.Properties[constants.GDCHConfigJSONField]
	if gdchConfigStr == "" {
		return nil, fmt.Errorf("%s required in secret", constants.GDCHConfigJSONField)
	}

	serviceAccountStr := h.config.Properties[constants.ServiceAccountJSONField]
	if serviceAccountStr == "" {
		return nil, fmt.Errorf("%s required in secret", constants.ServiceAccountJSONField)
	}

	gdcConfig := &gdcclient.OrgClusterConfig{}
	if err := json.Unmarshal([]byte(gdchConfigStr), gdcConfig); err != nil {
		return nil, fmt.Errorf("failed to parse GDC Config %w", err)
	}

	serviceAccount := &auth.ServiceAccount{}
	if err := json.Unmarshal([]byte(serviceAccountStr), serviceAccount); err != nil {
		return nil, fmt.Errorf("failed to parse service account %w", err)
	}

	var err error
	h.client, err = gdcclient.Get(gdcConfig, serviceAccount, sharedScheme)
	if err != nil {
		return nil, fmt.Errorf("failed to create Global kubeClient %w", err)
	}

	h.project = serviceAccount.Project
	return h, nil
}

func ensureQuotedText(v string) string {
	if _, err := strconv.Unquote(v); err != nil {
		v = strconv.Quote(v)
	}
	return v
}

func trimTrailingDot(s string) string {
	if s != "" && s[len(s)-1] == '.' {
		return s[:len(s)-1]
	}
	return s
}
