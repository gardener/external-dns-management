/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. h file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package cloudflare

import (
	"fmt"
	"github.com/cloudflare/cloudflare-go"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/hashicorp/go-multierror"
)

type Handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig
	cache  provider.ZoneCache
	api    *cloudflare.API
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *c,
	}

	apiToken, err := c.GetRequiredProperty("CLOUDFLARE_API_TOKEN", "apiToken")
	if err != nil {
		return nil, err
	}
	//email, err := c.GetRequiredProperty("CLOUDFLARE_API_EMAIL", "email")
	//if err != nil {
	//	return nil, err
	//}

	h.api, err = cloudflare.NewWithAPIToken(apiToken)
	if err != nil {
		return nil, fmt.Errorf("Creating Cloudflare object with api token failed: %s", err.Error())
	}

	// dummy call to check authentication
	_, err = h.api.ListZones()
	if err != nil {
		return nil, fmt.Errorf("Authentication test to Cloudflare with api token failed. Please check secret for DNSProvider. Details: %s", err.Error())
	}

	h.cache, err = provider.NewZoneCache(c.CacheConfig, c.Metrics, nil, h.getZones, h.getZoneState)
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

func (h *Handler) getZones(cache provider.ZoneCache) (provider.DNSHostedZones, error) {
	zones := provider.DNSHostedZones{}
	results, err := h.api.ListZones()
	h.config.Metrics.AddRequests("ZonesClient_ListComplete", 1)
	if err != nil {
		return nil, fmt.Errorf("Listing DNS zones failed. Details: %s", err.Error())
	}

	for _, zone := range results {
		// Check if the current api token has access to the zone.
		_, err := h.api.ZoneDetails(zone.ID)
		if err == nil {
			hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zone.ID, dns.NormalizeHostname(zone.Name), "", []string{}, false)
			zones = append(zones, hostedZone)
		}
	}

	return zones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	results, err := h.api.DNSRecords(zone.Id(), cloudflare.DNSRecord{})

	h.config.Metrics.AddRequests("RecordSetsClient_ListAllByDNSZoneComplete", 1)
	if err != nil {
		return nil, fmt.Errorf("Listing DNS zones failed. Details: %s", err.Error())
	}

	for _, entry := range results {
		switch entry.Type {
		case dns.RS_A, dns.RS_CNAME, dns.RS_TXT:
			rs := dns.NewRecordSet(entry.Type, int64(entry.TTL), nil)
			rs.Add(&dns.Record{Value: entry.Content})
			dnssets.AddRecordSetFromProvider(entry.Name, rs)
		}
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ReportZoneStateConflict(zone provider.DNSHostedZone, err error) bool {
	return h.cache.ReportZoneStateConflict(zone, err)
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
		status, name, dnsRecords := exec.buildRecordSet(r)
		if status == bs_empty || status == bs_dryrun {
			continue
		} else if status == bs_invalidType {
			err := fmt.Errorf("Unexpected record type: %s", r.Type)
			if r.Done != nil {
				r.Done.SetInvalid(err)
			}
			continue
		} else if status == bs_invalidName {
			err := fmt.Errorf("Unexpected dns name: %s", name)
			if r.Done != nil {
				r.Done.SetInvalid(err)
			}
			continue
		}

		var recordFailed int
		var recordErrors error

		for _, dnsRecord := range *dnsRecords {
			err := exec.apply(r.Action, dnsRecord, h.config.Metrics)
			if err != nil {
				recordFailed++
				logger.Infof("Apply failed with %s", err.Error())
				recordErrors = multierror.Append(recordErrors, err)
			}
		}
		if recordFailed > 0 {
			failed++
			if r.Done != nil {
				r.Done.Failed(recordErrors)
			}
		} else {
			succeeded++
			if r.Done != nil {
				r.Done.Succeeded()
			}
		}
	}

	if h.config.DryRun {
		logger.Infof("no changes in dryrun mode for Cloudflare")
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
