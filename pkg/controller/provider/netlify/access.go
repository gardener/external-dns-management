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

package netlify

import (
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/netlify/open-api/go/models"
	"github.com/netlify/open-api/go/plumbing"
	"github.com/netlify/open-api/go/plumbing/operations"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Access interface {
	ListZones(consume func(zone models.DNSZone) (bool, error)) error
	ListRecords(zoneId string, consume func(record models.DNSRecord) (bool, error)) error

	raw.Executor
}

type access struct {
	client      operations.ClientService
	authInfo    runtime.ClientAuthInfoWriter
	metrics     provider.Metrics
	rateLimiter flowcontrol.RateLimiter
}

func ClientCredentials(apiToken string) runtime.ClientAuthInfoWriter {
	return runtime.ClientAuthInfoWriterFunc(func(r runtime.ClientRequest, _ strfmt.Registry) error {
		r.SetHeaderParam("User-Agent", "external-dns-manager")
		r.SetHeaderParam("Authorization", "Bearer "+apiToken)
		return nil
	})
}

func NewAccess(apiToken string, metrics provider.Metrics, rateLimiter flowcontrol.RateLimiter) (Access, error) {
	client := plumbing.Default.Operations
	clientCredentials := ClientCredentials(apiToken)
	return &access{client: client, authInfo: clientCredentials, metrics: metrics, rateLimiter: rateLimiter}, nil
}

func (this *access) ListZones(consume func(zone models.DNSZone) (bool, error)) error {
	this.metrics.AddRequests(provider.M_LISTZONES, 1)
	this.rateLimiter.Accept()
	results, err := this.client.GetDNSZones(nil, this.authInfo)
	if err != nil {
		return err
	}
	for _, z := range results.Payload {
		if cont, err := consume(*z); !cont || err != nil {
			return err
		}
	}
	return nil
}

func (this *access) ListRecords(zoneId string, consume func(record models.DNSRecord) (bool, error)) error {
	this.metrics.AddRequests(provider.M_LISTRECORDS, 1)
	this.rateLimiter.Accept()
	params := operations.NewGetDNSRecordsParams()
	params.ZoneID = zoneId
	results, err := this.client.GetDNSRecords(params, this.authInfo)
	if err != nil {
		return err
	}
	for _, z := range results.Payload {
		if cont, err := consume(*z); !cont || err != nil {
			return err
		}
	}
	return nil
}

func (this *access) CreateRecord(r raw.Record) error {
	a := r.(*Record)
	ttl := r.GetTTL()
	testTTL(&ttl)
	dnsRecord := models.DNSRecordCreate{
		Type:     r.GetType(),
		Hostname: r.GetDNSName(),
		Value:    r.GetValue(),
		TTL:      int64(ttl),
	}
	this.metrics.AddRequests(provider.M_CREATERECORDS, 1)
	this.rateLimiter.Accept()
	createParams := operations.NewCreateDNSRecordParams()
	createParams.SetZoneID(a.DNSZoneID)
	createParams.SetDNSRecord(&dnsRecord)
	_, err := this.client.CreateDNSRecord(createParams, this.authInfo)
	return err
}

func (this *access) UpdateRecord(r raw.Record) error {
	// Netlify does not support updating a record
	// Delete the existing record and re-create it
	err := this.DeleteRecord(r)
	if err != nil {
		return err
	}
	return this.CreateRecord(r)
}

func (this *access) DeleteRecord(r raw.Record) error {
	a := r.(*Record)
	deleteParams := operations.NewDeleteDNSRecordParams()
	deleteParams.SetZoneID(a.DNSZoneID)
	deleteParams.SetDNSRecordID(a.ID)
	this.metrics.AddRequests(provider.M_DELETERECORDS, 1)
	this.rateLimiter.Accept()
	_, err := this.client.DeleteDNSRecord(deleteParams, this.authInfo)
	return err
}

func (this *access) NewRecord(fqdn, rtype, value string, zone provider.DNSHostedZone, ttl int64) raw.Record {
	return (*Record)(&models.DNSRecord{
		Type:      rtype,
		Hostname:  fqdn,
		Value:     value,
		TTL:       int64(ttl),
		DNSZoneID: zone.Id(),
	})
}

func testTTL(ttl *int) {
	if *ttl < 1 {
		*ttl = 1
	}
}
