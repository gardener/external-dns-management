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
	"strings"

	"github.com/cloudflare/cloudflare-go"
	"github.com/gardener/controller-manager-library/pkg/logger"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig
	cache  provider.ZoneCache
	access Access
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	var err error

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *c,
	}

	apiToken, err := c.GetRequiredProperty("CLOUDFLARE_API_TOKEN", "apiToken")
	if err != nil {
		return nil, err
	}
	//Email would be necessary in case of API KEY based auth which I am not supporting now. API token is more secure anyway
	//email, err := c.GetRequiredProperty("CLOUDFLARE_API_EMAIL", "email")
	//if err != nil {
	//	return nil, err
	//}

	access, err := NewAccess(apiToken, c.Metrics, c.RateLimiter)
	if err != nil {
		return nil, err
	}

	h.access = access

	h.cache, err = provider.NewZoneCache(*c.CacheConfig.CopyWithDisabledZoneStateCache(), c.Metrics, nil, h.getZones, h.getZoneState)
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
	rawZones := []cloudflare.Zone{}
	{
		f := func(zone cloudflare.Zone) (bool, error) {
			rawZones = append(rawZones, zone)
			return true, nil
		}
		err := h.access.ListZones(f)
		if err != nil {
			return nil, err
		}
	}

	zones := provider.DNSHostedZones{}

	for _, z := range rawZones {
		forwarded := []string{}
		f := func(r cloudflare.DNSRecord) (bool, error) {
			if r.Type == dns.RS_NS {
				name := r.Name
				if name != z.Name {
					forwarded = append(forwarded, name)
				}
			}
			return true, nil
		}
		err := h.access.ListRecords(z.ID, f)
		if err != nil {
			if checkAccessForbidden(err) {
				// It is possible to deny access to certain zones in the account
				// As a result, z zone should not be appended to the hosted zones
				continue
			}
			return nil, err
		}
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), z.ID, z.Name, z.ID, forwarded, false)
		zones = append(zones, hostedZone)
	}

	return zones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone, forceUpdate bool) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone, forceUpdate)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	state := raw.NewState()

	f := func(r cloudflare.DNSRecord) (bool, error) {
		a := (*Record)(&r)
		state.AddRecord(a)
		return true, nil
	}
	err := h.access.ListRecords(zone.Key(), f)
	if err != nil {
		return nil, err
	}
	state.CalculateDNSSets()
	return state, nil
}

func (h *Handler) ReportZoneStateConflict(zone provider.DNSHostedZone, err error) bool {
	return h.cache.ReportZoneStateConflict(zone, err)
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := raw.ExecuteRequests(logger, &h.config, h.access, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func checkAccessForbidden(err error) bool {
	if err != nil && strings.Contains(err.Error(), "403") {
		return true
	}
	return false
}
