// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package alicloud

import (
	alidns "github.com/alibabacloud-go/alidns-20150109/v4/client"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"k8s.io/utils/ptr"

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
	blockedZones := h.config.Options.GetBlockedZones()
	rawZones := []*alidns.DescribeDomainsResponseBodyDomainsDomain{}
	{
		f := func(zone *alidns.DescribeDomainsResponseBodyDomainsDomain) (bool, error) {
			domainID := ptr.Deref(zone.DomainId, "")
			if blockedZones.Contains(domainID) {
				h.config.Logger.Infof("ignoring blocked zone id: %s", domainID)
			} else {
				rawZones = append(rawZones, zone)
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
		for _, z := range rawZones {
			domainID := ptr.Deref(z.DomainId, "")
			domainName := ptr.Deref(z.DomainName, "")
			hostedZone := provider.NewDNSHostedZone(h.ProviderType(), domainID, domainName, domainName, false)
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

	f := func(record *alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord) (bool, error) {
		r := (*Record)(record)
		state.AddRecord(r)
		return true, nil
	}
	err := h.access.ListRecords(zone.Id().ID, zone.Key(), f)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "list records failed")
	}
	state.CalculateDNSSets()
	return state, nil
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
