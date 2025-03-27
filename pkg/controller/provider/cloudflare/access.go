// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cloudflare

import (
	"github.com/cloudflare/cloudflare-go"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Access interface {
	ListZones(consume func(zone cloudflare.Zone) (bool, error)) error
	ListRecords(zoneId string, consume func(record cloudflare.DNSRecord) (bool, error)) error

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

func (this *access) ListZones(consume func(zone cloudflare.Zone) (bool, error)) error {
	this.metrics.AddGenericRequests(provider.M_LISTZONES, 1)
	this.rateLimiter.Accept()
	results, err := this.API.ListZones()
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

func (this *access) ListRecords(zoneId string, consume func(record cloudflare.DNSRecord) (bool, error)) error {
	return this.listRecords(zoneId, consume, cloudflare.DNSRecord{})
}

func (this *access) listRecords(zoneId string, consume func(record cloudflare.DNSRecord) (bool, error),
	record cloudflare.DNSRecord,
) error {
	this.metrics.AddZoneRequests(zoneId, provider.M_LISTRECORDS, 1)
	this.rateLimiter.Accept()
	results, err := this.DNSRecords(zoneId, record)
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

func (this *access) CreateRecord(r raw.Record, zone provider.DNSHostedZone) error {
	ttl := r.GetTTL()
	testTTL(&ttl)
	dnsRecord := cloudflare.DNSRecord{
		Type:    r.GetType(),
		Name:    r.GetDNSName(),
		Content: r.GetValue(),
		TTL:     int(ttl),
		ZoneID:  zone.Id().ID,
	}
	this.metrics.AddZoneRequests(zone.Id().ID, provider.M_CREATERECORDS, 1)
	this.rateLimiter.Accept()
	_, err := this.CreateDNSRecord(zone.Id().ID, dnsRecord)
	return err
}

func (this *access) UpdateRecord(r raw.Record, zone provider.DNSHostedZone) error {
	ttl := r.GetTTL()
	testTTL(&ttl)
	dnsRecord := cloudflare.DNSRecord{
		Type:    r.GetType(),
		Name:    r.GetDNSName(),
		Content: r.GetValue(),
		TTL:     int(ttl),
		ZoneID:  zone.Id().ID,
	}
	this.metrics.AddZoneRequests(zone.Id().ID, provider.M_UPDATERECORDS, 1)
	this.rateLimiter.Accept()
	err := this.UpdateDNSRecord(zone.Id().ID, r.GetId(), dnsRecord)
	return err
}

func (this *access) DeleteRecord(r raw.Record, zone provider.DNSHostedZone) error {
	this.metrics.AddZoneRequests(zone.Id().ID, provider.M_DELETERECORDS, 1)
	this.rateLimiter.Accept()
	err := this.DeleteDNSRecord(zone.Id().ID, r.GetId())
	return err
}

func (this *access) NewRecord(fqdn, rtype, value string, zone provider.DNSHostedZone, ttl int64) raw.Record {
	return (*Record)(&cloudflare.DNSRecord{
		Type:    rtype,
		Name:    fqdn,
		Content: value,
		TTL:     int(ttl),
		ZoneID:  zone.Id().ID,
	})
}

func (this *access) GetRecordSet(dnsName, rtype string, zone provider.DNSHostedZone) (raw.RecordSet, error) {
	rs := raw.RecordSet{}
	consume := func(record cloudflare.DNSRecord) (bool, error) {
		a := (*Record)(&record)
		rs = append(rs, a)
		return true, nil
	}

	err := this.listRecords(zone.Id().ID, consume, cloudflare.DNSRecord{Type: rtype, Name: dnsName})
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
