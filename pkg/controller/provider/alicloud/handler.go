/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package alicloud

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
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

	accessKeyID, err := c.GetRequiredProperty("ACCESS_KEY_ID", "accessKeyID")
	if err != nil {
		return nil, err
	}
	accessKeySecret, err := c.GetRequiredProperty("ACCESS_KEY_SECRET", "accessKeySecret")
	if err != nil {
		return nil, err
	}

	access, err := NewAccess(accessKeyID, accessKeySecret, c.Metrics, c.RateLimiter)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "Creating alicloud access with client credentials failed")
	}

	h.access = access

	h.cache, err = c.ZoneCacheFactory.CreateZoneCache(provider.CacheZonesOnly, c.Metrics, h.getZones, h.getZoneState)
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
	blockedZones := h.config.Options.AdvancedOptions.GetBlockedZones()
	raw := []alidns.Domain{}
	{
		f := func(zone alidns.Domain) (bool, error) {
			if blockedZones.Contains(zone.DomainId) {
				h.config.Logger.Infof("ignoring blocked zone id: %s", zone.DomainId)
			} else {
				raw = append(raw, zone)
			}
			return true, nil
		}
		err := h.access.ListDomains(f)
		if err != nil {
			return nil, perrs.WrapAsHandlerError(err, "list domains failed")
		}
	}

	zones := provider.DNSHostedZones{}
	{
		for _, z := range raw {
			hostedZone := provider.NewDNSHostedZone(h.ProviderType(), z.DomainId, z.DomainName, z.DomainName, false)
			zones = append(zones, hostedZone)
		}
	}

	return zones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, _ provider.ZoneCache) (provider.DNSZoneState, error) {
	state := raw.NewState()

	f := func(r alidns.Record) (bool, error) {
		a := (*Record)(&r)
		state.AddRecord(a)
		// fmt.Printf("**** found %s %s: %s\n", a.GetType(), a.GetDNSName(), a.GetValue() )
		return true, nil
	}
	err := h.access.ListRecords(zone.Id().ID, zone.Key(), f)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "list records failed")
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

func (h *Handler) GetRecordSet(zone provider.DNSHostedZone, dnsName, recordType string) (provider.DedicatedRecordSet, error) {
	rs, err := h.access.GetRecordSet(dnsName, recordType, zone)
	if err != nil {
		return nil, err
	}
	d := provider.DedicatedRecordSet{}
	for _, r := range rs {
		d = append(d, r)
	}
	return d, nil
}

func (h *Handler) CreateOrUpdateRecordSet(logger logger.LogContext, zone provider.DNSHostedZone, old, new provider.DedicatedRecordSet) error {
	err := h.DeleteRecordSet(logger, zone, old)
	if err != nil {
		return err
	}
	for _, r := range new {
		r0 := h.access.NewRecord(r.GetDNSName(), r.GetType(), r.GetValue(), zone, int64(r.GetTTL()))
		err = h.access.CreateRecord(r0, zone)
		if err != nil {
			return err
		}
	}
	return err
}

func (h *Handler) DeleteRecordSet(_ logger.LogContext, zone provider.DNSHostedZone, rs provider.DedicatedRecordSet) error {
	for _, r := range rs {
		if r.(*Record).GetId() != "" {
			err := h.access.DeleteRecord(r.(*Record), zone)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
