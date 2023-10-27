/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. h file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package azureprivate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	azure "github.com/Azure/azure-sdk-for-go/services/privatedns/mgmt/2018-09-01/privatedns"
	"github.com/gardener/controller-manager-library/pkg/logger"

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
	zonesClient   *azure.PrivateZonesClient
	recordsClient *azure.RecordSetsClient
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *c,
	}

	h.ctx = c.Context

	subscriptionID, authorizer, err := utils.GetSubscriptionIDAndAuthorizer(c)
	if err != nil {
		return nil, err
	}

	zonesClient := azure.NewPrivateZonesClient(subscriptionID)
	recordsClient := azure.NewRecordSetsClient(subscriptionID)

	zonesClient.Authorizer = authorizer
	recordsClient.Authorizer = authorizer

	// dummy call to check authentication
	var one int32 = 1
	ctx := context.TODO()
	h.config.RateLimiter.Accept()
	_, err = zonesClient.List(ctx, &one)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "Authentication test to Azure with client credentials failed. Please check secret for DNSProvider.")
	}

	h.zonesClient = &zonesClient
	h.recordsClient = &recordsClient

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
	results, err := h.zonesClient.ListComplete(h.ctx, nil)
	h.config.Metrics.AddGenericRequests(provider.M_LISTZONES, 1)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "Listing DNS zones failed")
	}

	ctx := context.Background()
	blockedZones := h.config.Options.AdvancedOptions.GetBlockedZones()
	for results.NotDone() {

		item := results.Value()

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
			// ResourceGroup needed for requests to Azure. Remember by adding to Id. Split by calling splitZoneid().
			hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zoneID, dns.NormalizeHostname(*item.Name), "", []string{}, true)

			zones = append(zones, hostedZone)
		}

		if err := results.NextWithContext(ctx); err != nil {
			return nil, err
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
	results, err := h.recordsClient.ListComplete(h.ctx, resourceGroup, zoneName, nil, "")
	h.config.Metrics.AddZoneRequests(zone.Id().ID, provider.M_LISTRECORDS, 1)
	if err != nil {
		return nil, perrs.WrapfAsHandlerError(err, "Listing DNS zone state for zone %s failed", zoneName)
	}

	ctx := context.Background()
	count := 0
	for results.NotDone() {
		count++
		item := results.Value()
		// We expect recordName.DNSZone. However Azure only return recordName . Reverse is dropZoneName() needed for calls to Azure
		fullName := fmt.Sprintf("%s.%s", *item.Name, zoneName)

		if item.ARecords != nil {
			rs := dns.NewRecordSet(dns.RS_A, *item.TTL, nil)
			for _, record := range *item.ARecords {
				rs.Add(&dns.Record{Value: *record.Ipv4Address})
			}
			dnssets.AddRecordSetFromProvider(fullName, rs)
		}

		if item.AaaaRecords != nil {
			rs := dns.NewRecordSet(dns.RS_AAAA, *item.TTL, nil)
			for _, record := range *item.AaaaRecords {
				rs.Add(&dns.Record{Value: *record.Ipv6Address})
			}
			dnssets.AddRecordSetFromProvider(fullName, rs)
		}

		if item.CnameRecord != nil {
			rs := dns.NewRecordSet(dns.RS_CNAME, *item.TTL, nil)
			rs.Add(&dns.Record{Value: *item.CnameRecord.Cname})
			dnssets.AddRecordSetFromProvider(fullName, rs)
		}

		if item.TxtRecords != nil {
			rs := dns.NewRecordSet(dns.RS_TXT, *item.TTL, nil)
			for _, record := range *item.TxtRecords {
				quoted := strings.Join(*record.Value, "\n")
				// AzureDNS stores values unquoted, but it is expected to be quoted in dns.Record
				if len(quoted) > 0 && quoted[0] != '"' && quoted[len(quoted)-1] != '"' {
					quoted = strconv.Quote(quoted)
				}
				rs.Add(&dns.Record{Value: quoted})
			}
			dnssets.AddRecordSetFromProvider(fullName, rs)
		}

		if err := results.NextWithContext(ctx); err != nil {
			return nil, err
		}
	}
	pages := count / 100
	if pages > 0 {
		h.config.Metrics.AddZoneRequests(zone.Id().ID, provider.M_PLISTRECORDS, count/100)
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
