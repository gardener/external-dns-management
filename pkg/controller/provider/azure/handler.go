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

package azure

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	azure "github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"
	"github.com/Azure/go-autorest/autorest/azure/auth"

	"github.com/gardener/controller-manager-library/pkg/logger"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
)

type Handler struct {
	provider.DefaultDNSHandler
	config        provider.DNSHandlerConfig
	cache         provider.ZoneCache
	ctx           context.Context
	zonesClient   *azure.ZonesClient
	recordsClient *azure.RecordSetsClient
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *c,
	}

	h.ctx = c.Context

	subscriptionID, err := c.GetRequiredProperty("AZURE_SUBSCRIPTION_ID", "subscriptionID")
	if err != nil {
		return nil, err
	}
	// see https://docs.microsoft.com/en-us/go/azure/azure-sdk-go-authorization
	clientID, err := c.GetRequiredProperty("AZURE_CLIENT_ID", "clientID")
	if err != nil {
		return nil, err
	}
	clientSecret, err := c.GetRequiredProperty("AZURE_CLIENT_SECRET", "clientSecret")
	if err != nil {
		return nil, err
	}
	tenantID, err := c.GetRequiredProperty("AZURE_TENANT_ID", "tenantID")
	if err != nil {
		return nil, err
	}

	authorizer, err := auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID).Authorizer()
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "Creating Azure authorizer with client credentials failed")
	}

	zonesClient := azure.NewZonesClient(subscriptionID)
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

	h.cache, err = provider.NewZoneCache(c.CacheConfig, c.Metrics, nil, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return h, nil
}

var re = regexp.MustCompile("/resourceGroups/([^/]+)/")

func (h *Handler) Release() {
	h.cache.Release()
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) getZones(cache provider.ZoneCache) (provider.DNSHostedZones, error) {
	zones := provider.DNSHostedZones{}
	h.config.RateLimiter.Accept()
	results, err := h.zonesClient.ListComplete(h.ctx, nil)
	h.config.Metrics.AddGenericRequests(provider.M_LISTZONES, 1)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "Listing DNS zones failed")
	}

	for ; results.NotDone(); results.Next() {
		item := results.Value()

		submatches := re.FindStringSubmatch(*item.ID)
		if len(submatches) != 2 {
			logger.Warnf("Unexpected DNS Zone ID: '%s'. Skipping zone", *item.ID)
			continue
		}
		resourceGroup := submatches[1]

		forwarded := h.collectForwardedSubzones(resourceGroup, *item.Name)

		// ResourceGroup needed for requests to Azure. Remember by adding to Id. Split by calling splitZoneid().
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), makeZoneID(resourceGroup, *item.Name), dns.NormalizeHostname(*item.Name), "", forwarded, false)

		zones = append(zones, hostedZone)
	}

	return zones, nil
}

func (h *Handler) collectForwardedSubzones(resourceGroup, zoneName string) []string {
	forwarded := []string{}
	// There should only few NS entries. Therefore no paging is performed for simplicity.
	var top int32 = 1000
	h.config.RateLimiter.Accept()
	result, err := h.recordsClient.ListByType(h.ctx, resourceGroup, zoneName, azure.NS, &top, "")
	zoneID := makeZoneID(resourceGroup, zoneName)
	h.config.Metrics.AddZoneRequests(zoneID, provider.M_LISTRECORDS, 1)
	if err != nil {
		logger.Infof("Failed fetching NS records for %s: %s", zoneName, err.Error())
		// just ignoring it
		return forwarded
	}

	for _, item := range result.Values() {
		if *item.Name != "@" && item.NsRecords != nil && len(*item.NsRecords) > 0 {
			fullDomainName := *item.Name + "." + zoneName
			forwarded = append(forwarded, fullDomainName)
		}
	}
	return forwarded
}

func makeZoneID(resourceGroup, zoneName string) string {
	return resourceGroup + "/" + zoneName
}

func splitZoneid(zoneid string) (string, string) {
	parts := strings.Split(zoneid, "/")
	if len(parts) != 2 {
		return "", zoneid
	}
	return parts[0], parts[1]
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone, forceUpdate bool) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone, forceUpdate)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	resourceGroup, zoneName := splitZoneid(zone.Id())
	h.config.RateLimiter.Accept()
	results, err := h.recordsClient.ListAllByDNSZoneComplete(h.ctx, resourceGroup, zoneName, nil, "")
	h.config.Metrics.AddZoneRequests(zone.Id(), provider.M_LISTRECORDS, 1)
	if err != nil {
		return nil, perrs.WrapfAsHandlerError(err, "Listing DNS zone state for zone %s failed", zoneName)
	}

	count := 0
	for ; results.NotDone(); results.Next() {
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
	}
	pages := count / 100
	if pages > 0 {
		h.config.Metrics.AddZoneRequests(zone.Id(), provider.M_PLISTRECORDS, count/100)
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

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	resourceGroup, zoneName := splitZoneid(zone.Id())
	exec := NewExecution(logger, h, resourceGroup, zoneName)

	var succeeded, failed int
	for _, r := range reqs {
		status, recordType, rset := exec.buildRecordSet(r)
		if status == bs_empty || status == bs_dryrun {
			continue
		} else if status == bs_invalidType {
			err := fmt.Errorf("Unexpected record type: %s", r.Type)
			if r.Done != nil {
				r.Done.SetInvalid(err)
			}
			continue
		} else if status == bs_invalidName {
			err := fmt.Errorf("Unexpected dns name: %s", *rset.Name)
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
