// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cloudflare

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go/v6"
	cloudflaredns "github.com/cloudflare/cloudflare-go/v6/dns"
	"github.com/cloudflare/cloudflare-go/v6/option"
	cloudflarezones "github.com/cloudflare/cloudflare-go/v6/zones"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Access interface {
	ListZones(ctx context.Context, consume func(zone cloudflarezones.Zone) (bool, error)) error
	ListRecords(ctx context.Context, zoneId string, consume func(record cloudflaredns.RecordResponse) (bool, error)) error

	raw.Executor
}

type access struct {
	client      *cloudflare.Client
	metrics     provider.Metrics
	rateLimiter flowcontrol.RateLimiter
}

func NewAccess(apiToken string, metrics provider.Metrics, rateLimiter flowcontrol.RateLimiter) (Access, error) {
	client := cloudflare.NewClient(option.WithAPIToken(apiToken))
	return &access{client: client, metrics: metrics, rateLimiter: rateLimiter}, nil
}

func (a *access) ListZones(ctx context.Context, consume func(zone cloudflarezones.Zone) (bool, error)) error {
	a.metrics.AddGenericRequests(provider.M_LISTZONES, 1)
	a.rateLimiter.Accept()
	iter := a.client.Zones.ListAutoPaging(ctx, cloudflarezones.ZoneListParams{})
	for iter.Next() {
		zone := iter.Current()
		if cont, err := consume(zone); !cont || err != nil {
			return err
		}
		// assume paging size of 100 to limit rate. The real page size is not exposed by the iterator.
		if iter.Index()%100 == 99 {
			a.metrics.AddGenericRequests(provider.M_PLISTZONES, 1)
			a.rateLimiter.Accept()
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("could not list zones: %w", err)
	}
	return nil
}

func (a *access) ListRecords(ctx context.Context, zoneId string, consume func(record cloudflaredns.RecordResponse) (bool, error)) error {
	return a.listRecords(ctx, zoneId, consume, nil, nil)
}

func (a *access) listRecords(ctx context.Context, zoneId string, consume func(record cloudflaredns.RecordResponse) (bool, error),
	recordListType *cloudflaredns.RecordListParamsType, name *cloudflaredns.RecordListParamsName,
) error {
	a.metrics.AddZoneRequests(zoneId, provider.M_LISTRECORDS, 1)
	params := cloudflaredns.RecordListParams{
		ZoneID: cloudflare.F(zoneId),
	}
	if recordListType != nil {
		params.Type = cloudflare.F(*recordListType)
	}
	if name != nil {
		params.Name = cloudflare.F(*name)
	}
	iter := a.client.DNS.Records.ListAutoPaging(ctx, params)
	for iter.Next() {
		record := iter.Current()
		if cont, err := consume(record); !cont || err != nil {
			return err
		}
		// assume paging size of 100 to limit rate. The real page size is not exposed by the iterator.
		if iter.Index()%100 == 99 {
			a.metrics.AddGenericRequests(provider.M_PLISTRECORDS, 1)
			a.rateLimiter.Accept()
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("could not list records: %w", err)
	}
	return nil
}

func (a *access) CreateRecord(ctx context.Context, r raw.Record, zone provider.DNSHostedZone) error {
	body, err := toNewRecordParamsBody(r)
	if err != nil {
		return err
	}
	a.metrics.AddZoneRequests(zone.Id().ID, provider.M_CREATERECORDS, 1)
	a.rateLimiter.Accept()
	_, err = a.client.DNS.Records.New(ctx, cloudflaredns.RecordNewParams{
		ZoneID: cloudflare.F(zone.Id().ID),
		Body:   body,
	})
	return err
}

func (a *access) UpdateRecord(ctx context.Context, r raw.Record, zone provider.DNSHostedZone) error {
	body, err := toUpdateRecordParamsBody(r)
	if err != nil {
		return err
	}
	a.metrics.AddZoneRequests(zone.Id().ID, provider.M_UPDATERECORDS, 1)
	a.rateLimiter.Accept()
	_, err = a.client.DNS.Records.Update(ctx, r.GetId(), cloudflaredns.RecordUpdateParams{
		ZoneID: cloudflare.F(zone.Id().ID),
		Body:   body,
	})
	return err
}

func (a *access) DeleteRecord(ctx context.Context, r raw.Record, zone provider.DNSHostedZone) error {
	a.metrics.AddZoneRequests(zone.Id().ID, provider.M_DELETERECORDS, 1)
	a.rateLimiter.Accept()
	_, err := a.client.DNS.Records.Delete(ctx, r.GetId(), cloudflaredns.RecordDeleteParams{ZoneID: cloudflare.F(zone.Id().ID)})
	return err
}

func (a *access) NewRecord(fqdn, rtype, value string, _ provider.DNSHostedZone, ttl int64) raw.Record {
	return (*Record)(&cloudflaredns.RecordResponse{
		Type:    cloudflaredns.RecordResponseType(rtype),
		Name:    fqdn,
		Content: value,
		TTL:     cloudflaredns.TTL(ttl),
	})
}

func (a *access) GetRecordSet(ctx context.Context, dnsName, rtype string, zone provider.DNSHostedZone) (raw.RecordSet, error) {
	rs := raw.RecordSet{}
	consume := func(record cloudflaredns.RecordResponse) (bool, error) {
		r := (*Record)(&record)
		rs = append(rs, r)
		return true, nil
	}

	err := a.listRecords(ctx,
		zone.Id().ID,
		consume,
		ptr.To(cloudflaredns.RecordListParamsType(rtype)),
		&cloudflaredns.RecordListParamsName{
			Exact: cloudflare.F(dnsName),
		})
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func toNewRecordParamsBody(r raw.Record) (cloudflaredns.RecordNewParamsBodyUnion, error) {
	ttl := r.GetTTL()
	testTTL(&ttl)

	switch r.GetType() {
	case dns.RS_A:
		return cloudflaredns.ARecordParam{
			Name:    cloudflare.F(r.GetDNSName()),
			Type:    cloudflare.F(cloudflaredns.ARecordTypeA),
			TTL:     cloudflare.F(cloudflaredns.TTL(ttl)),
			Content: cloudflare.F(r.GetValue()),
		}, nil
	case dns.RS_AAAA:
		return cloudflaredns.AAAARecordParam{
			Name:    cloudflare.F(r.GetDNSName()),
			Type:    cloudflare.F(cloudflaredns.AAAARecordTypeAAAA),
			TTL:     cloudflare.F(cloudflaredns.TTL(ttl)),
			Content: cloudflare.F(r.GetValue()),
		}, nil
	case dns.RS_CNAME:
		return cloudflaredns.CNAMERecordParam{
			Name:    cloudflare.F(r.GetDNSName()),
			Type:    cloudflare.F(cloudflaredns.CNAMERecordTypeCNAME),
			TTL:     cloudflare.F(cloudflaredns.TTL(ttl)),
			Content: cloudflare.F(r.GetValue()),
		}, nil
	case dns.RS_TXT:
		return cloudflaredns.TXTRecordParam{
			Name:    cloudflare.F(r.GetDNSName()),
			Type:    cloudflare.F(cloudflaredns.TXTRecordTypeTXT),
			TTL:     cloudflare.F(cloudflaredns.TTL(ttl)),
			Content: cloudflare.F(r.GetValue()),
		}, nil
	default:
		return nil, fmt.Errorf("record type %q not supported", r.GetType())
	}
}

func toUpdateRecordParamsBody(r raw.Record) (cloudflaredns.RecordUpdateParamsBodyUnion, error) {
	ttl := r.GetTTL()
	testTTL(&ttl)

	switch r.GetType() {
	case dns.RS_A:
		return cloudflaredns.ARecordParam{
			Name:    cloudflare.F(r.GetDNSName()),
			Type:    cloudflare.F(cloudflaredns.ARecordTypeA),
			TTL:     cloudflare.F(cloudflaredns.TTL(ttl)),
			Content: cloudflare.F(r.GetValue()),
		}, nil
	case dns.RS_AAAA:
		return cloudflaredns.AAAARecordParam{
			Name:    cloudflare.F(r.GetDNSName()),
			Type:    cloudflare.F(cloudflaredns.AAAARecordTypeAAAA),
			TTL:     cloudflare.F(cloudflaredns.TTL(ttl)),
			Content: cloudflare.F(r.GetValue()),
		}, nil
	case dns.RS_CNAME:
		return cloudflaredns.CNAMERecordParam{
			Name:    cloudflare.F(r.GetDNSName()),
			Type:    cloudflare.F(cloudflaredns.CNAMERecordTypeCNAME),
			TTL:     cloudflare.F(cloudflaredns.TTL(ttl)),
			Content: cloudflare.F(r.GetValue()),
		}, nil
	case dns.RS_TXT:
		return cloudflaredns.TXTRecordParam{
			Name:    cloudflare.F(r.GetDNSName()),
			Type:    cloudflare.F(cloudflaredns.TXTRecordTypeTXT),
			TTL:     cloudflare.F(cloudflaredns.TTL(ttl)),
			Content: cloudflare.F(r.GetValue()),
		}, nil
	default:
		return nil, fmt.Errorf("record type %q not supported", r.GetType())
	}
}

func testTTL(ttl *int64) {
	if *ttl < 120 {
		*ttl = 1
	}
}
