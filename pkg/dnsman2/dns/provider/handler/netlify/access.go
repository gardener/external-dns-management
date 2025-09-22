// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package netlify

import (
	"context"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/netlify/open-api/go/models"
	"github.com/netlify/open-api/go/plumbing"
	"github.com/netlify/open-api/go/plumbing/operations"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/raw"
)

type accessItf interface {
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

func clientCredentials(apiToken string) runtime.ClientAuthInfoWriter {
	return runtime.ClientAuthInfoWriterFunc(func(r runtime.ClientRequest, _ strfmt.Registry) error {
		_ = r.SetHeaderParam("User-Agent", "external-dns-manager")
		_ = r.SetHeaderParam("Authorization", "Bearer "+apiToken)
		return nil
	})
}

func newAccess(apiToken string, metrics provider.Metrics, rateLimiter flowcontrol.RateLimiter) (accessItf, error) {
	client := plumbing.Default.Operations
	clientCredentials := clientCredentials(apiToken)
	return &access{client: client, authInfo: clientCredentials, metrics: metrics, rateLimiter: rateLimiter}, nil
}

func (this *access) ListZones(consume func(zone models.DNSZone) (bool, error)) error {
	this.metrics.AddGenericRequests(provider.MetricsRequestTypeListZones, 1)
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

func (this *access) ListRecords(zoneID string, consume func(record models.DNSRecord) (bool, error)) error {
	this.metrics.AddZoneRequests(zoneID, provider.MetricsRequestTypeListRecords, 1)
	this.rateLimiter.Accept()
	params := operations.NewGetDNSRecordsParams()
	params.ZoneID = zoneID
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

func (this *access) CreateRecord(_ context.Context, r raw.Record, zone provider.DNSHostedZone) error {
	a := r.(*Record)
	ttl := r.GetTTL()
	testTTL(&ttl)
	dnsRecord := models.DNSRecordCreate{
		Type:     r.GetType(),
		Hostname: r.GetDNSName(),
		Value:    r.GetValue(),
		TTL:      ttl,
	}
	this.metrics.AddZoneRequests(zone.ZoneID().ID, provider.MetricsRequestTypeCreateRecords, 1)
	this.rateLimiter.Accept()
	createParams := operations.NewCreateDNSRecordParams()
	createParams.SetZoneID(a.DNSZoneID)
	createParams.SetDNSRecord(&dnsRecord)
	_, err := this.client.CreateDNSRecord(createParams, this.authInfo)
	return err
}

func (this *access) UpdateRecord(ctx context.Context, r raw.Record, zone provider.DNSHostedZone) error {
	// Netlify does not support updating a record
	// Delete the existing record and re-create it
	err := this.DeleteRecord(ctx, r, zone)
	if err != nil {
		return err
	}
	return this.CreateRecord(ctx, r, zone)
}

func (this *access) DeleteRecord(_ context.Context, r raw.Record, zone provider.DNSHostedZone) error {
	a := r.(*Record)
	deleteParams := operations.NewDeleteDNSRecordParams()
	deleteParams.SetZoneID(a.DNSZoneID)
	deleteParams.SetDNSRecordID(a.ID)
	this.metrics.AddZoneRequests(zone.ZoneID().ID, provider.MetricsRequestTypeDeleteRecords, 1)
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
		DNSZoneID: zone.ZoneID().ID,
	})
}

func (this *access) GetRecordList(_ context.Context, dnsName, rtype string, zone provider.DNSHostedZone) (raw.RecordList, []*dns.RoutingPolicy, error) {
	rl := raw.RecordList{}
	consume := func(record models.DNSRecord) (bool, error) {
		a := (*Record)(&record)
		if a.Type == rtype && a.Hostname == dnsName {
			rl = append(rl, a)
		}
		return true, nil
	}

	// no filtering provided by API, we have to list complete zone and filter
	err := this.ListRecords(zone.ZoneID().ID, consume)
	if err != nil {
		return nil, nil, err
	}
	return rl, nil, nil
}

func testTTL(ttl *int64) {
	if *ttl < 1 {
		*ttl = 1
	}
}
