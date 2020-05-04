/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
	this.metrics.AddRequests(provider.M_LISTZONES, 1)
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
	this.metrics.AddRequests(provider.M_LISTRECORDS, 1)
	this.rateLimiter.Accept()
	results, err := this.DNSRecords(zoneId, cloudflare.DNSRecord{})
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

func (this *access) CreateRecord(r raw.Record) error {
	a := r.(*Record)
	ttl := r.GetTTL()
	testTTL(&ttl)
	dnsRecord := cloudflare.DNSRecord{
		Type:    r.GetType(),
		Name:    r.GetDNSName(),
		Content: r.GetValue(),
		TTL:     ttl,
		ZoneID:  a.ZoneID,
	}
	this.metrics.AddRequests(provider.M_CREATERECORDS, 1)
	this.rateLimiter.Accept()
	_, err := this.CreateDNSRecord(a.ZoneID, dnsRecord)
	return err
}

func (this *access) UpdateRecord(r raw.Record) error {
	a := r.(*Record)
	ttl := r.GetTTL()
	testTTL(&ttl)
	dnsRecord := cloudflare.DNSRecord{
		Type:    r.GetType(),
		Name:    r.GetDNSName(),
		Content: r.GetValue(),
		TTL:     ttl,
		ZoneID:  a.ZoneID,
	}
	this.metrics.AddRequests(provider.M_UPDATERECORDS, 1)
	this.rateLimiter.Accept()
	err := this.UpdateDNSRecord(a.ZoneID, r.GetId(), dnsRecord)
	return err
}

func (this *access) DeleteRecord(r raw.Record) error {
	a := r.(*Record)
	this.metrics.AddRequests(provider.M_DELETERECORDS, 1)
	this.rateLimiter.Accept()
	err := this.DeleteDNSRecord(a.ZoneID, r.GetId())
	return err
}

func (this *access) NewRecord(fqdn, rtype, value string, zone provider.DNSHostedZone, ttl int64) raw.Record {
	return (*Record)(&cloudflare.DNSRecord{
		Type:    rtype,
		Name:    fqdn,
		Content: value,
		TTL:     int(ttl),
		ZoneID:  zone.Id(),
	})
}

func testTTL(ttl *int) {
	if *ttl < 120 {
		*ttl = 1
	}
}
