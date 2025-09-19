// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cloudflare

import (
	"context"

	cloudflaredns "github.com/cloudflare/cloudflare-go/v6/dns"
	cloudflarezones "github.com/cloudflare/cloudflare-go/v6/zones"
	"github.com/gardener/controller-manager-library/pkg/logger"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig
	cache  provider.ZoneCache
	access Access
	ctx    context.Context
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	var err error

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *c,
		ctx:               c.Context,
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
	blockedZones := h.config.Options.GetBlockedZones()
	rawZones := []cloudflarezones.Zone{}
	{
		f := func(zone cloudflarezones.Zone) (bool, error) {
			if blockedZones.Contains(zone.ID) {
				h.config.Logger.Infof("ignoring blocked zone id: %s", zone.ID)
			} else {
				rawZones = append(rawZones, zone)
			}
			return true, nil
		}
		err := h.access.ListZones(h.ctx, f)
		if err != nil {
			return nil, err
		}
	}

	zones := provider.DNSHostedZones{}

	for _, z := range rawZones {
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), z.ID, z.Name, z.ID, false)
		zones = append(zones, hostedZone)
	}

	return zones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, _ provider.ZoneCache) (provider.DNSZoneState, error) {
	state := raw.NewState()

	f := func(r cloudflaredns.RecordResponse) (bool, error) {
		a := (*Record)(&r)
		state.AddRecord(a)
		return true, nil
	}
	err := h.access.ListRecords(h.ctx, zone.Id().ID, f)
	if err != nil {
		return nil, err
	}
	state.CalculateDNSSets()
	return state, nil
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := raw.ExecuteRequests(h.ctx, logger, &h.config, h.access, zone, state, reqs, nil)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) GetRecordSet(zone provider.DNSHostedZone, dnsName, recordType string) (provider.DedicatedRecordSet, error) {
	rs, err := h.access.GetRecordSet(h.ctx, dnsName, recordType, zone)
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
		err = h.access.CreateRecord(h.ctx, r0, zone)
		if err != nil {
			return err
		}
	}
	return err
}

func (h *Handler) DeleteRecordSet(_ logger.LogContext, zone provider.DNSHostedZone, rs provider.DedicatedRecordSet) error {
	for _, r := range rs {
		if r.(*Record).GetId() != "" {
			err := h.access.DeleteRecord(h.ctx, r.(*Record), zone)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
