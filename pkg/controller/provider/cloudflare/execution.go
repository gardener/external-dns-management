/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. exec file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use exec file except in compliance with the License.
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
	"strings"

	azure "github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Change struct {
	rs   *azure.RecordSet
	Done provider.DoneHandler
}

type Execution struct {
	logger.LogContext
	handler *Handler
	zone    provider.DNSHostedZone

	changes map[string][]*Change
}

func NewExecution(logger logger.LogContext, h *Handler, zone provider.DNSHostedZone) *Execution {
	return &Execution{LogContext: logger, handler: h, zone: zone, changes: map[string][]*Change{}}
}

type buildStatus int

const (
	bs_ok          buildStatus = 0
	bs_invalidType buildStatus = 1
	bs_empty       buildStatus = 2
	bs_dryrun      buildStatus = 3
	bs_invalidName buildStatus = 4
)

// Shorten DnsEntry-dnsName from record name + .DNSZone to record name only: e.g www2.test6227.ml to www2
func dropZoneName(dnsName, zoneName string) (string, bool) {
	end := len(dnsName) - len(zoneName) - 1
	if end <= 0 || !strings.HasSuffix(dnsName, zoneName) || dnsName[end] != '.' {
		return dnsName, false
	}
	return dnsName[:end], true
}

func (exec *Execution) buildRecordSet(req *provider.ChangeRequest) (buildStatus, string, *[]cloudflare.DNSRecord) {
	var dnsset *dns.DNSSet
	switch req.Action {
	case provider.R_CREATE, provider.R_UPDATE:
		dnsset = req.Addition
	case provider.R_DELETE:
		dnsset = req.Deletion
	}

	name, rset := dns.MapToProvider(req.Type, dnsset, exec.zone.Domain())
	name, ok := dropZoneName(name, exec.zone.Domain())
	if !ok {
		var records []cloudflare.DNSRecord
		records = append(records, cloudflare.DNSRecord{Name: name})
		return bs_invalidName, name, &records
	}

	if len(rset.Records) == 0 {
		return bs_empty, "", nil
	}

	exec.Infof("Desired %s: %s record set %s[%s] with TTL %d: %s", req.Action, rset.Type, name, exec.zone.Domain(), rset.TTL, rset.RecordString())
	return exec.buildMappedRecordSet(name, rset)
}

func (exec *Execution) buildMappedRecordSet(name string, rset *dns.RecordSet) (buildStatus, string, *[]cloudflare.DNSRecord) {
	var recordType = rset.Type

	var TTL = 1
	var records []cloudflare.DNSRecord

	switch rset.Type {
	case dns.RS_A, dns.RS_CNAME, dns.RS_TXT:
		for _, r := range rset.Records {
			records = append(records, cloudflare.DNSRecord{
				Type:    recordType,
				Name:    name,
				Content: r.Value,
				TTL:     TTL,
			})
		}
	default:
		return bs_invalidType, "", nil
	}

	return bs_ok, name, &records
}

func (exec *Execution) apply(action string, record cloudflare.DNSRecord, metrics provider.Metrics) error {
	var err error
	switch action {
	case provider.R_CREATE:
		err = exec.create(record, metrics)
	case provider.R_UPDATE:
		err = exec.update(record, metrics)
	case provider.R_DELETE:
		err = exec.delete(record, metrics)
	}
	return err
}

func (exec *Execution) create(record cloudflare.DNSRecord, metrics provider.Metrics) error {
	_, err := exec.handler.api.CreateDNSRecord(exec.zone.Id(), record)
	metrics.AddRequests("RecordSetsClient_Create", 1)
	return err
}

func (exec *Execution) update(record cloudflare.DNSRecord, metrics provider.Metrics) error {
	err := exec.handler.api.UpdateDNSRecord(exec.zone.Id(), record.ID, record)
	metrics.AddRequests("RecordSetsClient_Update", 1)
	return err
}

func (exec *Execution) delete(record cloudflare.DNSRecord, metrics provider.Metrics) error {
	err := exec.handler.api.DeleteDNSRecord(exec.zone.Id(), record.ID)
	metrics.AddRequests("RecordSetsClient_Delete", 1)
	return err
}
