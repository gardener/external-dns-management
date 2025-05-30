// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cloudflare

import (
	"context"

	"github.com/cloudflare/cloudflare-go"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Access interface {
	ListZones(ctx context.Context, consume func(zone cloudflare.Zone) (bool, error)) error
	ListRecords(ctx context.Context, zoneId string, consume func(record cloudflare.DNSRecord) (bool, error)) error

	raw.Executor
}

type access struct {
	*cloudflare.API
	metrics     provider.Metrics
	rateLimiter flowcontrol.RateLimiter
}

func NewAccess(apiToken string, metrics provider.Metrics, rateLimiter flowcontrol.RateLimiter) (Access, error) {
	api, err := cloudflare.NewWithAPIToken(apiToken)
	if err != nil {
		return nil, err
	}
	return &access{API: api, metrics: metrics, rateLimiter: rateLimiter}, nil
}

func (a *access) ListZones(ctx context.Context, consume func(zone cloudflare.Zone) (bool, error)) error {
	a.metrics.AddGenericRequests(provider.M_LISTZONES, 1)
	a.rateLimiter.Accept()
	results, err := a.API.ListZones(ctx)
	if err != nil {
		return err
	}
	for _, z := range results {
		if cont, err := consume(z); !cont || err != nil {
			return err
		}
	}
	return nil
}

func (a *access) ListRecords(ctx context.Context, zoneId string, consume func(record cloudflare.DNSRecord) (bool, error)) error {
	return a.listRecords(ctx, zoneId, consume, cloudflare.DNSRecord{})
}

func (a *access) listRecords(ctx context.Context, zoneId string, consume func(record cloudflare.DNSRecord) (bool, error),
	record cloudflare.DNSRecord,
) error {
	a.metrics.AddZoneRequests(zoneId, provider.M_LISTRECORDS, 1)
	var (
		resultInfo = &cloudflare.ResultInfo{
			Page:    1,
			PerPage: 100,
		}
		err     error
		results []cloudflare.DNSRecord
	)
	for {
		a.metrics.AddZoneRequests(zoneId, provider.M_PLISTRECORDS, 1)
		a.rateLimiter.Accept()
		results, resultInfo, err = a.ListDNSRecords(ctx, toResourceContainer(zoneId), cloudflare.ListDNSRecordsParams{
			Type:       record.Type,
			Name:       record.Name,
			ResultInfo: *resultInfo,
		})
		if err != nil {
			return err
		}
		for _, z := range results {
			if cont, err := consume(z); !cont || err != nil {
				return err
			}
		}
		if resultInfo.Page >= resultInfo.TotalPages {
			break
		}
		resultInfo.Page++
	}
	return nil
}

func (a *access) CreateRecord(ctx context.Context, r raw.Record, zone provider.DNSHostedZone) error {
	ttl := r.GetTTL()
	testTTL(&ttl)
	dnsRecord := cloudflare.CreateDNSRecordParams{
		Type:    r.GetType(),
		Name:    r.GetDNSName(),
		Content: r.GetValue(),
		TTL:     int(ttl),
	}
	a.metrics.AddZoneRequests(zone.Id().ID, provider.M_CREATERECORDS, 1)
	a.rateLimiter.Accept()
	_, err := a.CreateDNSRecord(ctx, toResourceContainer(zone.Id().ID), dnsRecord)
	return err
}

func (a *access) UpdateRecord(ctx context.Context, r raw.Record, zone provider.DNSHostedZone) error {
	ttl := r.GetTTL()
	testTTL(&ttl)
	dnsRecord := cloudflare.UpdateDNSRecordParams{
		ID:      r.GetId(),
		Type:    r.GetType(),
		Name:    r.GetDNSName(),
		Content: r.GetValue(),
		TTL:     int(ttl),
	}
	a.metrics.AddZoneRequests(zone.Id().ID, provider.M_UPDATERECORDS, 1)
	a.rateLimiter.Accept()
	_, err := a.UpdateDNSRecord(ctx, toResourceContainer(zone.Id().ID), dnsRecord)
	return err
}

func (a *access) DeleteRecord(ctx context.Context, r raw.Record, zone provider.DNSHostedZone) error {
	a.metrics.AddZoneRequests(zone.Id().ID, provider.M_DELETERECORDS, 1)
	a.rateLimiter.Accept()
	err := a.DeleteDNSRecord(ctx, toResourceContainer(zone.Id().ID), r.GetId())
	return err
}

func (a *access) NewRecord(fqdn, rtype, value string, _ provider.DNSHostedZone, ttl int64) raw.Record {
	return (*Record)(&cloudflare.DNSRecord{
		Type:    rtype,
		Name:    fqdn,
		Content: value,
		TTL:     int(ttl),
	})
}

func (a *access) GetRecordSet(ctx context.Context, dnsName, rtype string, zone provider.DNSHostedZone) (raw.RecordSet, error) {
	rs := raw.RecordSet{}
	consume := func(record cloudflare.DNSRecord) (bool, error) {
		r := (*Record)(&record)
		rs = append(rs, r)
		return true, nil
	}

	err := a.listRecords(ctx, zone.Id().ID, consume, cloudflare.DNSRecord{Type: rtype, Name: dnsName})
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func testTTL(ttl *int64) {
	if *ttl < 120 {
		*ttl = 1
	}
}

func toResourceContainer(zoneId string) *cloudflare.ResourceContainer {
	return &cloudflare.ResourceContainer{
		Identifier: zoneId,
		Type:       cloudflare.ZoneType,
	}
}
