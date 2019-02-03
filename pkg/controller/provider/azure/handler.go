/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. h file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use h file except in compliance with the License.
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
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"

	azure "github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-03-01-preview/dns"
	"github.com/Azure/go-autorest/autorest/azure/auth"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Handler struct {
	config provider.DNSHandlerConfig
	ctx    context.Context

	zonesClient   *azure.ZonesClient
	recordsClient *azure.RecordSetsClient
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(logger logger.LogContext, config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {

	h := &Handler{
		config: *config,
	}

	h.ctx = config.Context

	subscriptionID := h.config.Properties["AZURE_SUBSCRIPTION_ID"]
	if subscriptionID == "" {
		return nil, fmt.Errorf("'AZURE_SUBSCRIPTION_ID' required in secret")
	}
	// see https://docs.microsoft.com/en-us/go/azure/azure-sdk-go-authorization
	clientID := h.config.Properties["AZURE_CLIENT_ID"]
	if clientID == "" {
		return nil, fmt.Errorf("'AZURE_CLIENT_ID' required in secret")
	}
	clientSecret := h.config.Properties["AZURE_CLIENT_SECRET"]
	if clientSecret == "" {
		return nil, fmt.Errorf("'AZURE_CLIENT_SECRET' required in secret")
	}
	tenantID := h.config.Properties["AZURE_TENANT_ID"]
	if tenantID == "" {
		return nil, fmt.Errorf("'AZURE_TENANT_ID' required in secret")
	}

	authorizer, err := auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID).Authorizer()
	if err != nil {
		return nil, fmt.Errorf("Creating Azure authorizer with client credentials failed: %s", err.Error())
	}

	zonesClient := azure.NewZonesClient(subscriptionID)
	recordsClient := azure.NewRecordSetsClient(subscriptionID)

	zonesClient.Authorizer = authorizer
	recordsClient.Authorizer = authorizer
	// dummy call to check authentication
	var one int32 = 1
	_, err = zonesClient.List(h.ctx, &one)
	if err != nil {
		return nil, fmt.Errorf("Authentication test to Azure with client credentials failed. Please check secret for DNSProvider. Details: %s", err.Error())
	}

	h.zonesClient = &zonesClient
	h.recordsClient = &recordsClient
	return h, nil
}

var re = regexp.MustCompile("/resourceGroups/([^/]+)/")

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	zones := provider.DNSHostedZones{}

	results, err := h.zonesClient.ListComplete(h.ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("Listing DNS zones failed. Details: %s", err.Error())
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
		hostedZone := provider.NewDNSHostedZone(
			resourceGroup + "/" + *item.Name,
			dns.NormalizeHostname(*item.Name),
			"",
			forwarded,
		)

		zones = append(zones, hostedZone)
	}

	return zones, nil
}

func (h *Handler) collectForwardedSubzones(resourceGroup, zoneName string) []string {
	forwarded := []string{}
	// There should only few NS entries. Therefore no paging is performed for simplicity.
	var top int32 = 1000
	result, err := h.recordsClient.ListByType(h.ctx, resourceGroup, zoneName, azure.NS, &top, "")
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

func splitZoneid(zoneid string) (string, string) {
	parts := strings.Split(zoneid, "/")
	if len(parts) != 2 {
		return "", zoneid
	}
	return parts[0], parts[1]
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	resourceGroup, zoneName := splitZoneid(zone.Id())
	results, err := h.recordsClient.ListAllByDNSZoneComplete(h.ctx, resourceGroup, zoneName, nil, "")
	if err != nil {
		return nil, fmt.Errorf("Listing DNS zones failed. Details: %s", err.Error())
	}

	for ; results.NotDone(); results.Next() {
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
				rs.Add(&dns.Record{Value: strings.Join(*record.Value, "\n")})
			}
			dnssets.AddRecordSetFromProvider(fullName, rs)
		}
	}
	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
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

		err := exec.apply(r.Action, recordType, rset)
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
	}

	return nil
}
