// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package azureprivate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/controller/provider/azure/utils"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
)

type Handler struct {
	provider.DefaultDNSHandler
	config        provider.DNSHandlerConfig
	cache         provider.ZoneCache
	ctx           context.Context
	zonesClient   *armprivatedns.PrivateZonesClient
	recordsClient *armprivatedns.RecordSetsClient
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *c,
	}

	h.ctx = c.Context

	subscriptionID, tc, err := utils.GetSubscriptionIDAndCredentials(c)
	if err != nil {
		return nil, err
	}
	opts, err := utils.GetDefaultAzureClientOpts(c)
	if err != nil {
		return nil, err
	}

	zonesClient, err := armprivatedns.NewPrivateZonesClient(subscriptionID, tc, opts)
	if err != nil {
		return nil, err
	}
	recordsClient, err := armprivatedns.NewRecordSetsClient(subscriptionID, tc, opts)
	if err != nil {
		return nil, err
	}

	// dummy call to check authentication
	h.config.RateLimiter.Accept()
	_, err = zonesClient.NewListPager(&armprivatedns.PrivateZonesClientListOptions{Top: ptr.To[int32](1)}).NextPage(h.ctx)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "Authentication test to Azure with client credentials failed. Please check secret for DNSProvider.")
	}

	h.zonesClient = zonesClient
	h.recordsClient = recordsClient

	h.cache, err = c.ZoneCacheFactory.CreateZoneCache(provider.CacheZoneState, c.Metrics, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (h *Handler) Release() {
	h.cache.Release()
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) getZones(_ provider.ZoneCache) (provider.DNSHostedZones, error) {
	zones := provider.DNSHostedZones{}
	h.config.RateLimiter.Accept()

	blockedZones := h.config.Options.AdvancedOptions.GetBlockedZones()
	pager := h.zonesClient.NewListPager(nil)
	for pager.More() {
		h.config.Metrics.AddGenericRequests(provider.M_LISTZONES, 1)
		page, err := pager.NextPage(h.ctx)
		if err != nil {
			if err != nil {
				return nil, perrs.WrapAsHandlerError(err, "Listing DNS zones failed")
			}
		}

		for _, item := range page.Value {
			var zoneID string
			resourceGroup, err := utils.ExtractResourceGroup(*item.ID)
			if err != nil {
				logger.Warnf("skipping zone: %s", err)
			} else {
				zoneID = utils.MakeZoneID(resourceGroup, *item.Name)
				if blockedZones.Contains(zoneID) {
					h.config.Logger.Infof("ignoring blocked zone id: %s", zoneID)
					zoneID = ""
				}
			}

			if zoneID != "" {
				// ResourceGroup needed for requests to Azure. Remember by adding to Id. Split by calling SplitZoneID().
				hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zoneID, dns.NormalizeHostname(*item.Name), "", false)

				zones = append(zones, hostedZone)
			}
		}
	}

	return zones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, _ provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	resourceGroup, zoneName := utils.SplitZoneID(zone.Id().ID)
	h.config.RateLimiter.Accept()

	h.config.Metrics.AddZoneRequests(zone.Id().ID, provider.M_LISTRECORDS, 1)
	pager := h.recordsClient.NewListPager(resourceGroup, zoneName, nil)
	for pager.More() {
		page, err := pager.NextPage(h.ctx)
		h.config.Metrics.AddZoneRequests(zone.Id().ID, provider.M_PLISTRECORDS, 1)
		if err != nil {
			return nil, perrs.WrapfAsHandlerError(err, "Listing DNS zone state for zone %s failed", zoneName)
		}
		for _, item := range page.Value {
			// We expect recordName.DNSZone. However Azure only return recordName . Reverse is dropZoneName() needed for calls to Azure
			fullName := fmt.Sprintf("%s.%s", *item.Name, zoneName)

			switch {
			case item.Properties.ARecords != nil:
				rs := dns.NewRecordSet(dns.RS_A, *item.Properties.TTL, nil)
				for _, record := range item.Properties.ARecords {
					rs.Add(&dns.Record{Value: *record.IPv4Address})
				}
				dnssets.AddRecordSetFromProvider(fullName, rs)
			case item.Properties.AaaaRecords != nil:
				rs := dns.NewRecordSet(dns.RS_AAAA, *item.Properties.TTL, nil)
				for _, record := range item.Properties.AaaaRecords {
					rs.Add(&dns.Record{Value: *record.IPv6Address})
				}
				dnssets.AddRecordSetFromProvider(fullName, rs)
			case item.Properties.CnameRecord != nil:
				rs := dns.NewRecordSet(dns.RS_CNAME, *item.Properties.TTL, nil)
				rs.Add(&dns.Record{Value: *item.Properties.CnameRecord.Cname})
				dnssets.AddRecordSetFromProvider(fullName, rs)
			case item.Properties.TxtRecords != nil:
				rs := dns.NewRecordSet(dns.RS_TXT, *item.Properties.TTL, nil)
				for _, record := range item.Properties.TxtRecords {
					values := make([]string, len(record.Value))
					for i, value := range record.Value {
						values[i] = *value
					}
					quoted := strings.Join(values, "\n")
					// AzureDNS stores values unquoted, but it is expected to be quoted in dns.Record
					if len(quoted) > 0 && quoted[0] != '"' && quoted[len(quoted)-1] != '"' {
						quoted = strconv.Quote(quoted)
					}
					rs.Add(&dns.Record{Value: quoted})
				}
				dnssets.AddRecordSetFromProvider(fullName, rs)
			}
		}
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ReportZoneStateConflict(zone provider.DNSHostedZone, err error) bool {
	return h.cache.ReportZoneStateConflict(zone, err)
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, _ provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	resourceGroup, zoneName := utils.SplitZoneID(zone.Id().ID)
	exec := NewExecution(logger, h, resourceGroup, zoneName)

	var succeeded, failed int
	for _, r := range reqs {
		status, recordType, rset := exec.buildRecordSet(r)
		switch status {
		case bs_empty:
			continue
		case bs_dryrun:
			continue
		case bs_invalidType:
			err := fmt.Errorf("Unexpected record type: %s", r.Type)
			if r.Done != nil {
				r.Done.SetInvalid(err)
			}
			continue
		case bs_invalidName:
			err := fmt.Errorf("Unexpected dns name: %s", *rset.Name)
			if r.Done != nil {
				r.Done.SetInvalid(err)
			}
			continue
		case bs_invalidRoutingPolicy:
			err := fmt.Errorf("Routing policies not supported for " + TYPE_CODE)
			if r.Done != nil {
				r.Done.SetInvalid(err)
			}
			continue
		}

		err := exec.apply(r.Action, recordType, rset, h.config.Metrics)
		if err != nil {
			failed++
			logger.Infof("Apply failed with %s", err.Error())
			if r.Done != nil {
				r.Done.Failed(err)
			}
		} else {
			succeeded++
			if r.Done != nil {
				r.Done.Succeeded()
			}
		}
	}

	if h.config.DryRun {
		logger.Infof("no changes in dryrun mode for Azure")
		return nil
	}

	if succeeded > 0 {
		logger.Infof("Succeeded updates for records in zone %s: %d", zoneName, succeeded)
	}
	if failed > 0 {
		logger.Infof("Failed updates for records in zone %s: %d", zoneName, failed)
		return fmt.Errorf("%d changes failed", failed)
	}

	return nil
}
