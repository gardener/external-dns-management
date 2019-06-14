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

package openstack

import (
	"context"
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gophercloud/gophercloud/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/openstack/dns/v2/zones"
)

type Handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig
	cache  provider.ZoneCache
	ctx    context.Context

	client designateClientInterface
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(logger logger.LogContext, config *provider.DNSHandlerConfig, metrics provider.Metrics) (provider.DNSHandler, error) {
	authConfig, err := readAuthConfig(config)
	if err != nil {
		return nil, err
	}

	serviceClient, err := createDesignateServiceClient(logger, authConfig)
	if err != nil {
		return nil, err
	}

	h := Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *config,
		ctx:               config.Context,
		client:            designateClient{serviceClient: serviceClient, metrics: metrics},
	}

	h.cache, err = provider.NewZoneCache(config.CacheConfig, metrics, nil, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return &h, nil
}

func readConfigProperty(config *provider.DNSHandlerConfig, key string, altKey string) (value string, err error) {
	value = config.Properties[key]
	if value == "" && altKey != "" {
		value = config.Properties[altKey]
	}
	if value == "" {
		alt := ""
		if altKey != "" {
			alt = fmt.Sprintf(" or '%s'", altKey)
		}
		err = fmt.Errorf("'%s'%s required in secret", key, alt)
	}
	return
}

func readAuthConfig(config *provider.DNSHandlerConfig) (*authConfig, error) {
	authURL, err := readConfigProperty(config, "OS_AUTH_URL", "")
	if err != nil {
		return nil, err
	}
	username, err := readConfigProperty(config, "OS_USERNAME", "username")
	if err != nil {
		return nil, err
	}
	domainName, err := readConfigProperty(config, "OS_DOMAIN_NAME", "domainName")
	if err != nil {
		return nil, err
	}
	password, err := readConfigProperty(config, "OS_PASSWORD", "password")
	if err != nil {
		return nil, err
	}
	projectName, err := readConfigProperty(config, "OS_PROJECT_NAME", "tenantName")
	if err != nil {
		return nil, err
	}
	// optional restriction to region
	regionName := config.Properties["OS_REGION_NAME"]

	authConfig := authConfig{AuthURL: authURL, Username: username, Password: password,
		DomainName: domainName, ProjectName: projectName, RegionName: regionName}

	return &authConfig, nil
}

func (h *Handler) Release() {
	h.cache.Release()
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) getZones(cache provider.ZoneCache) (provider.DNSHostedZones, error) {
	hostedZones := provider.DNSHostedZones{}

	zoneHandler := func(zone *zones.Zone) error {
		forwarded := h.collectForwardedSubzones(zone)

		hostedZone := provider.NewDNSHostedZone(
			h.ProviderType(),
			zone.ID,
			dns.NormalizeHostname(zone.Name),
			"",
			forwarded,
		)
		hostedZones = append(hostedZones, hostedZone)
		return nil
	}

	if err := h.client.ForEachZone(zoneHandler); err != nil {
		return nil, fmt.Errorf("listing DNS zones failed. Details: %s", err.Error())
	}

	return hostedZones, nil
}

func (h *Handler) collectForwardedSubzones(zone *zones.Zone) []string {
	forwarded := []string{}

	recordSetHandler := func(recordSet *recordsets.RecordSet) error {
		if recordSet.Type == "NS" && recordSet.Name != zone.Name && len(recordSet.Records) > 0 {
			fullDomainName := dns.NormalizeHostname(recordSet.Name)
			forwarded = append(forwarded, fullDomainName)
		}
		return nil
	}

	if err := h.client.ForEachRecordSetFilterByTypeAndName(zone.ID, "NS", "", recordSetHandler); err != nil {
		logger.Infof("Failed fetching NS records for %s: %s", zone.Name, err.Error())
		// just ignoring it
		return forwarded
	}

	return forwarded
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	recordSetHandler := func(recordSet *recordsets.RecordSet) error {
		switch recordSet.Type {
		case dns.RS_A, dns.RS_CNAME, dns.RS_TXT:
			rs := dns.NewRecordSet(recordSet.Type, int64(recordSet.TTL), nil)
			for _, record := range recordSet.Records {
				value := record
				if recordSet.Type == dns.RS_CNAME {
					value = dns.NormalizeHostname(value)
				}
				rs.Add(&dns.Record{Value: value})
			}
			dnssets.AddRecordSetFromProvider(recordSet.Name, rs)
		}
		return nil
	}

	if err := h.client.ForEachRecordSet(zone.Id(), recordSetHandler); err != nil {
		return nil, fmt.Errorf("Listing DNS zones failed for %s. Details: %s", zone.Id(), err.Error())
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h, zone)

	var succeeded, failed int
	for _, r := range reqs {
		status, rset := exec.buildRecordSet(r)
		if status == bsEmpty || status == bsDryRun {
			continue
		}

		err := exec.apply(r.Action, rset)
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
		logger.Infof("no changes in dryrun mode for OpenStack")
		return nil
	}

	if succeeded > 0 {
		logger.Infof("Succeeded updates for records in zone %s: %d", zone.Domain(), succeeded)
	}
	if failed > 0 {
		logger.Infof("Failed updates for records in zone %s: %d", zone.Domain(), failed)
		return fmt.Errorf("%d changes failed", failed)
	}

	return nil
}
