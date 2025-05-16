// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package alicloud

import (
	"fmt"
	"strings"

	alidns "github.com/alibabacloud-go/alidns-20150109/v4/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

var (
	nullHost = "@"
	// defaultPageSize. According to the documentation, the maximum page size is 100.
	// @see https://www.alibabacloud.com/help/en/dns/api-alidns-2015-01-09-describedomains
	defaultPageSize int64 = 100
)

func GetDNSName(r alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord) string {
	rr := ptr.Deref(r.RR, "")
	domain := ptr.Deref(r.DomainName, "")
	if rr == nullHost {
		return domain
	}
	return rr + "." + domain
}

func GetRR(dnsname, domain string) string {
	if dnsname == domain {
		return nullHost
	}
	suffix := "." + domain
	if !strings.HasSuffix(dnsname, suffix) {
		panic(fmt.Sprintf("OOPS: dnsname %q does not match zone %q", dnsname, domain))
	}
	return dnsname[:len(dnsname)-len(suffix)]
}

type Access interface {
	ListDomains(consume func(domain *alidns.DescribeDomainsResponseBodyDomainsDomain) (bool, error)) error
	ListRecords(zoneID, domain string, consume func(record *alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord) (bool, error)) error

	raw.Executor
}

type access struct {
	client      *alidns.Client
	metrics     provider.Metrics
	rateLimiter flowcontrol.RateLimiter
}

func NewAccess(accessKeyId, accessKeySecret string, metrics provider.Metrics, rateLimiter flowcontrol.RateLimiter) (Access, error) {
	config := &openapi.Config{
		AccessKeyId:     &accessKeyId,
		AccessKeySecret: &accessKeySecret,
		RegionId:        ptr.To("cn-shanghai"),
	}
	client, err := alidns.NewClient(config)
	if err != nil {
		return nil, err
	}
	return &access{client, metrics, rateLimiter}, nil
}

func (a *access) nextPageNumber(pageNumber *int64, pageSize, totalCount int64) int64 {
	if pageNumber == nil {
		return 0
	}
	if (*pageNumber)*pageSize >= totalCount {
		return 0
	}
	return (*pageNumber) + 1
}

func (a *access) ListDomains(consume func(domain *alidns.DescribeDomainsResponseBodyDomainsDomain) (bool, error)) error {
	request := &alidns.DescribeDomainsRequest{}
	request.PageSize = ptr.To(defaultPageSize)
	var nextPage int64 = 1
	rt := provider.M_LISTZONES
	for {
		a.metrics.AddGenericRequests(rt, 1)
		rt = provider.M_PLISTZONES
		request.PageNumber = ptr.To(nextPage)
		a.rateLimiter.Accept()
		resp, err := a.client.DescribeDomains(request)
		if err != nil {
			return err
		}
		if resp.Body == nil || resp.Body.Domains == nil {
			return fmt.Errorf("unexpected empty response: %v", resp)
		}
		for _, d := range resp.Body.Domains.Domain {
			if cont, err := consume(d); !cont || err != nil {
				return err
			}
		}
		if nextPage = a.nextPageNumber(resp.Body.PageNumber, defaultPageSize, ptr.Deref(resp.Body.TotalCount, 0)); nextPage == 0 {
			return nil
		}
	}
}

func (a *access) ListRecords(zoneID, domain string, consume func(record *alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord) (bool, error)) error {
	return a.listRecords(zoneID, domain, consume, nil)
}

func (a *access) listRecords(zoneID, domain string, consume func(record *alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord) (bool, error),
	requestModifier func(request *alidns.DescribeDomainRecordsRequest),
) error {
	request := &alidns.DescribeDomainRecordsRequest{}
	request.DomainName = ptr.To(domain)
	if requestModifier != nil {
		requestModifier(request)
	}
	request.PageSize = ptr.To(defaultPageSize)
	var nextPage int64 = 1
	rt := provider.M_LISTRECORDS
	for {
		a.metrics.AddZoneRequests(zoneID, rt, 1)
		rt = provider.M_PLISTRECORDS
		request.PageNumber = ptr.To(nextPage)
		a.rateLimiter.Accept()
		resp, err := a.client.DescribeDomainRecords(request)
		if err != nil {
			return err
		}
		if resp.Body == nil || resp.Body.DomainRecords == nil {
			return fmt.Errorf("unexpected empty response: %v", resp)
		}
		for _, r := range resp.Body.DomainRecords.Record {
			if cont, err := consume(r); !cont || err != nil {
				return err
			}
		}
		if nextPage = a.nextPageNumber(resp.Body.PageNumber, defaultPageSize, ptr.Deref(resp.Body.TotalCount, 0)); nextPage == 0 {
			return nil
		}
	}
}

func (a *access) CreateRecord(record raw.Record, zone provider.DNSHostedZone) error {
	r := record.(*Record)
	req := &alidns.AddDomainRecordRequest{}
	req.DomainName = r.DomainName
	req.RR = r.RR
	req.Type = r.Type
	req.TTL = r.TTL
	req.Value = r.Value
	a.metrics.AddZoneRequests(zone.Id().ID, provider.M_UPDATERECORDS, 1)
	a.rateLimiter.Accept()
	resp, err := a.client.AddDomainRecord(req)
	if err != nil {
		return err
	}
	if record.GetSetIdentifier() != "" {
		if err := a.setRecordWeight(resp.Body.RecordId, record); err != nil {
			return fmt.Errorf("failed to set record weight: %w", err)
		}
	}
	return nil
}

func (a *access) setRecordWeight(recordId *string, record raw.Record) error {
	r := record.(*Record)
	req := &alidns.UpdateDomainRecordRemarkRequest{}
	req.RecordId = recordId
	if ptr.Deref(r.Remark, "") == deleteRemark {
		req.Remark = nil
	} else {
		req.Remark = ptr.To(routingPolicySetRemarkPrefix + record.GetSetIdentifier())
	}
	if _, err := a.client.UpdateDomainRecordRemark(req); err != nil {
		return err
	}

	req2 := &alidns.UpdateDNSSLBWeightRequest{}
	req2.RecordId = recordId
	req2.Weight = r.Weight
	if _, err := a.client.UpdateDNSSLBWeight(req2); err != nil {
		if !isSDKErrorWithCode(err, "DisableDNSSLB") {
			return err
		}

		// Need to enable weighted round-robin
		req3 := &alidns.SetDNSSLBStatusRequest{}
		req3.Type = r.Type
		req3.DomainName = r.DomainName
		req3.SubDomain = ptr.To(r.GetDNSName())
		if _, err := a.client.SetDNSSLBStatus(req3); err != nil {
			return fmt.Errorf("setDNSSLBStatus failed: %w", err)
		}
		if _, err := a.client.UpdateDNSSLBWeight(req2); err != nil {
			return fmt.Errorf("UpdateDNSSLBWeight failed on second try: %w", err)
		}
	}

	return nil
}

func (a *access) clearRecordWeight(recordId *string) error {
	req := &alidns.UpdateDomainRecordRemarkRequest{}
	req.RecordId = recordId
	if _, err := a.client.UpdateDomainRecordRemark(req); err != nil {
		return err
	}

	req2 := &alidns.UpdateDNSSLBWeightRequest{}
	req2.RecordId = recordId
	if _, err := a.client.UpdateDNSSLBWeight(req2); err != nil {
		return err
	}

	return nil
}

func (a *access) UpdateRecord(record raw.Record, zone provider.DNSHostedZone) error {
	r := record.(*Record)
	req := &alidns.UpdateDomainRecordRequest{}
	req.RecordId = r.RecordId
	req.RR = r.RR
	req.Type = r.Type
	req.TTL = r.TTL
	req.Value = r.Value
	a.metrics.AddZoneRequests(zone.Id().ID, provider.M_UPDATERECORDS, 1)
	a.rateLimiter.Accept()
	if _, err := a.client.UpdateDomainRecord(req); err != nil {
		if !isSDKErrorWithCode(err, "DomainRecordDuplicate") {
			return err
		}
	}
	if record.GetSetIdentifier() != "" {
		if err := a.setRecordWeight(r.RecordId, record); err != nil {
			return fmt.Errorf("failed to set record weight: %w", err)
		}
	} else if ptr.Deref(r.Remark, "") == deleteRemark {
		if err := a.clearRecordWeight(r.RecordId); err != nil {
			return fmt.Errorf("failed to clear record weight: %w", err)
		}
	}
	return nil
}

func (a *access) DeleteRecord(r raw.Record, zone provider.DNSHostedZone) error {
	req := &alidns.DeleteDomainRecordRequest{}
	req.RecordId = ptr.To(r.GetId())
	a.metrics.AddZoneRequests(zone.Id().ID, provider.M_UPDATERECORDS, 1)
	a.rateLimiter.Accept()
	_, err := a.client.DeleteDomainRecord(req)
	return err
}

func (a *access) GetRecordSet(dnsName, rtype string, zone provider.DNSHostedZone) (raw.RecordSet, error) {
	rr := GetRR(dnsName, zone.Domain())
	requestModifier := func(request *alidns.DescribeDomainRecordsRequest) {
		request.RRKeyWord = ptr.To(rr)
		request.TypeKeyWord = ptr.To(rtype)
	}

	rs := raw.RecordSet{}
	consume := func(record *alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord) (bool, error) {
		if ptr.Deref(record.RR, "") == rr {
			rs = append(rs, (*Record)(record))
		}
		return true, nil
	}

	err := a.listRecords(zone.Id().ID, zone.Domain(), consume, requestModifier)
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func (a *access) NewRecord(fqdn, rtype, value string, zone provider.DNSHostedZone, ttl int64) raw.Record {
	rr := GetRR(fqdn, zone.Domain())
	return (*Record)(&alidns.DescribeDomainRecordsResponseBodyDomainRecordsRecord{
		RR:         ptr.To(rr),
		Type:       ptr.To(rtype),
		Value:      ptr.To(value),
		DomainName: ptr.To(zone.Domain()),
		TTL:        ptr.To(ttl),
	})
}

func isSDKErrorWithCode(err error, code string) bool {
	if sdkErr := err.(*tea.SDKError); sdkErr != nil {
		return ptr.Deref(sdkErr.Code, "") == code
	}
	return false
}
