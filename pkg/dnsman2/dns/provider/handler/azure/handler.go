// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package azure

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"

	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	azutils "github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/azure/utils"
	dnsutils "github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig

	zonesClient   *armdns.ZonesClient
	recordsClient *armdns.RecordSetsClient
}

var _ provider.DNSHandler = &handler{}

// NewHandler constructs a new DNSHandler object.
func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	h := &handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *c,
	}

	subscriptionID, tokenCredential, err := azutils.GetSubscriptionIdAndCredentials(c)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "failed to get Azure subscriptionID and credentials")
	}
	opts, err := azutils.GetDefaultAzureClientOpts(c)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "failed to get default Azure client options")
	}

	zonesClient, err := armdns.NewZonesClient(subscriptionID, tokenCredential, opts)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "failed to create new Azure zones client")
	}
	recordsClient, err := armdns.NewRecordSetsClient(subscriptionID, tokenCredential, opts)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "failed to create new Azure record sets client")
	}

	err = h.checkAuthentication(zonesClient)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "authentication test to Azure with client credentials failed; please check the secret for the DNSProvider")
	}

	h.zonesClient = zonesClient
	h.recordsClient = recordsClient

	return h, nil
}

// Release releases the zone cache.
func (h *handler) Release() {
}

func (h *handler) getAdvancedOptions() config.AdvancedOptions {
	return h.config.GlobalConfig.ProviderAdvancedOptions[ProviderType]
}

// GetZones returns a list of hosted zones from the cache.
func (h *handler) GetZones(ctx context.Context) ([]provider.DNSHostedZone, error) {
	h.config.RateLimiter.Accept()
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var hostedZones []provider.DNSHostedZone
	pager := h.zonesClient.NewListPager(nil)
	for pager.More() {
		h.config.Metrics.AddGenericRequests(provider.MetricsRequestTypeListZones, 1)
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, perrs.WrapAsHandlerError(err, "failed to list zones")
		}

		for _, item := range page.Value {
			resourceGroup, err := azutils.ExtractResourceGroup(*item.ID)
			if err != nil {
				log.Error(err, "skipping zone", "id", *item.ID)
				continue
			}

			// The resource group is required for requests to Azure. We remember it by including it in the zone ID.
			// The zoneID has the form <resource-group>/<zone-id>. It can be split by calling SplitZoneID().
			zoneID, err := azutils.MakeZoneID(resourceGroup, *item.Name)
			if err != nil {
				log.Error(err, "skipping zone", "resourceGroup", resourceGroup, "zoneName", *item.Name)
				continue
			}
			if h.isBlockedZone(zoneID) {
				log.Info("ignoring blocked zone", "zone", zoneID)
				continue
			}
			hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zoneID, dns.NormalizeDomainName(*item.Name), "", false)
			hostedZones = append(hostedZones, hostedZone)
		}
	}

	return hostedZones, nil
}

func (h *handler) isBlockedZone(zoneID string) bool {
	for _, zone := range h.getAdvancedOptions().BlockedZones {
		if zone == zoneID {
			return true
		}
	}
	return false
}

func (h *handler) getLogFromContext(ctx context.Context) (logr.Logger, error) {
	log, err := logr.FromContext(ctx)
	if err != nil {
		return log, fmt.Errorf("failed to get logger from context: %w", err)
	}
	log = log.WithValues(
		"provider", h.ProviderType(),
	)
	return log, nil
}

// GetCustomQueryDNSFunc returns a custom DNS query function for the Azure DNS provider.
func (h *handler) GetCustomQueryDNSFunc(_ dns.ZoneInfo, factory dnsutils.QueryDNSFactoryFunc) (provider.CustomQueryDNSFunc, error) {
	defaultQueryFunc, err := factory()
	if err != nil {
		return nil, fmt.Errorf("failed to create default query function: %w", err)
	}
	return func(ctx context.Context, _ dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
		queryResult := defaultQueryFunc.Query(ctx, setName, recordType)
		return queryResult.RecordSet, queryResult.Err
	}, nil
}

// ExecuteRequests applies a given change request to a given hosted zone.
func (h *handler) ExecuteRequests(ctx context.Context, zone provider.DNSHostedZone, reqs provider.ChangeRequests) error {
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return err
	}

	resourceGroup, zoneName := azutils.SplitZoneID(zone.ZoneID().ID)
	exec := newExecution(log, h, zone, resourceGroup, zoneName)

	var (
		succeeded, failed int
		errs              []error
	)
	for _, r := range reqs.Updates {
		if err := exec.apply(ctx, reqs.Name, r); err != nil {
			failed++
			log.Error(err, "apply failed")
			errs = append(errs, err)
		} else {
			succeeded++
		}
	}

	if succeeded > 0 {
		log.Info("Succeeded updates for records", "zone", zone.ZoneID().ID, "count", succeeded)
	}
	if failed > 0 {
		log.Info("Failed updates for records", "zone", zone.ZoneID().ID, "count", failed)
		return fmt.Errorf("%d changes failed", failed)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to execute change requests for zone %s: %w", zone.ZoneID(), errors.Join(errs...))
	}
	return nil
}

func (h *handler) checkAuthentication(zonesClient *armdns.ZonesClient) error {
	h.config.RateLimiter.Accept()
	// dummy call to check authentication
	_, err := zonesClient.NewListPager(&armdns.ZonesClientListOptions{Top: ptr.To[int32](1)}).NextPage(context.Background())
	return err
}
