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
	"fmt"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
	"k8s.io/client-go/util/flowcontrol"
	"os"
	"strconv"
	"strings"
)

var nullHost = "@"
var defaultPageSize = 20

var alicloudDNSApiQPSEnv = "ALICLOUD_DNS_API_QPS"
var defaultAlicloudDNSApiQPS = 25.0

func GetDNSName(r alidns.Record) string {
	if r.RR == nullHost {
		return r.DomainName
	}
	return r.RR + "." + r.DomainName
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
	ListDomains(consume func(domain alidns.Domain) (bool, error)) error
	ListRecords(domain string, consume func(record alidns.Record) (bool, error)) error

	raw.Executor
}

type access struct {
	client      *alidns.Client
	metrics     provider.Metrics
	rateLimiter flowcontrol.RateLimiter
}

func NewAccess(accessKeyId, accessKeySecret string, metrics provider.Metrics) (Access, error) {
	client, err := alidns.NewClientWithAccessKey("cn-shanghai", accessKeyId, accessKeySecret)
	if err != nil {
		return nil, err
	}

	qps := defaultAlicloudDNSApiQPS
	qpsEnv := os.Getenv(alicloudDNSApiQPSEnv)
	if qpsEnv != "" {
		q, err := strconv.ParseFloat(qpsEnv, 32)
		if err == nil {
			logger.Warnf("Set QPS for Alicloud failed. Environment %s is invalid.", alicloudDNSApiQPSEnv)
		}
		qps = q
	}

	// To be conservative, let burst to be 1
	r := flowcontrol.NewTokenBucketRateLimiter(float32(qps), 1)
	return &access{client, metrics, r}, nil
}

func (this *access) nextPageNumber(pageNumber, pageSize, totalCount int) int {
	if pageNumber*pageSize >= totalCount {
		return 0
	}
	return pageNumber + 1
}

func (this *access) ListDomains(consume func(domain alidns.Domain) (bool, error)) error {
	request := alidns.CreateDescribeDomainsRequest()
	request.PageSize = requests.NewInteger(defaultPageSize)
	nextPage := 1
	rt := provider.M_LISTZONES
	for {
		this.metrics.AddRequests(rt, 1)
		rt = provider.M_PLISTZONES
		request.PageNumber = requests.NewInteger(nextPage)
		this.rateLimiter.Accept()
		resp, err := this.client.DescribeDomains(request)
		if err != nil {
			return err
		}
		for _, d := range resp.Domains.Domain {
			if cont, err := consume(d); !cont || err != nil {
				return err
			}
		}
		if nextPage = this.nextPageNumber(resp.PageNumber, defaultPageSize, resp.TotalCount); nextPage == 0 {
			return nil
		}
	}
}

func (this *access) ListRecords(domain string, consume func(record alidns.Record) (bool, error)) error {
	request := alidns.CreateDescribeDomainRecordsRequest()
	request.DomainName = domain
	request.PageSize = requests.NewInteger(defaultPageSize)
	nextPage := 1
	rt := provider.M_LISTRECORDS
	for {
		this.metrics.AddRequests(rt, 1)
		rt = provider.M_PLISTRECORDS
		request.PageNumber = requests.NewInteger(nextPage)
		this.rateLimiter.Accept()
		resp, err := this.client.DescribeDomainRecords(request)
		if err != nil {
			return err
		}
		for _, r := range resp.DomainRecords.Record {
			if cont, err := consume(r); !cont || err != nil {
				return err
			}
		}
		if nextPage = this.nextPageNumber(resp.PageNumber, defaultPageSize, resp.TotalCount); nextPage == 0 {
			return nil
		}
	}
}

func (this *access) CreateRecord(r raw.Record) error {
	a := r.(*Record)
	req := alidns.CreateAddDomainRecordRequest()
	req.DomainName = a.DomainName
	req.RR = a.RR
	req.Type = a.Type
	req.TTL = requests.NewInteger(a.TTL)
	req.Value = a.Value
	this.metrics.AddRequests(provider.M_UPDATERECORDS, 1)
	this.rateLimiter.Accept()
	_, err := this.client.AddDomainRecord(req)
	return err
}

func (this *access) UpdateRecord(r raw.Record) error {
	a := r.(*Record)
	req := alidns.CreateUpdateDomainRecordRequest()
	req.RecordId = a.RecordId
	req.RR = a.RR
	req.Type = a.Type
	req.TTL = requests.NewInteger(a.TTL)
	req.Value = a.Value
	this.metrics.AddRequests(provider.M_UPDATERECORDS, 1)
	this.rateLimiter.Accept()
	_, err := this.client.UpdateDomainRecord(req)
	return err
}

func (this *access) DeleteRecord(r raw.Record) error {
	req := alidns.CreateDeleteDomainRecordRequest()
	req.RecordId = r.GetId()
	this.metrics.AddRequests(provider.M_UPDATERECORDS, 1)
	this.rateLimiter.Accept()
	_, err := this.client.DeleteDomainRecord(req)
	return err
}

func (this *access) NewRecord(fqdn, rtype, value string, zone provider.DNSHostedZone, ttl int64) raw.Record {
	rr := GetRR(fqdn, zone.Domain())
	return (*Record)(&alidns.Record{RR: rr, Type: rtype, Value: value, DomainName: zone.Domain(), TTL: int(ttl)})
}
