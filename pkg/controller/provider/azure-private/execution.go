/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. exec file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package azureprivate

import (
	"strconv"

	azure "github.com/Azure/azure-sdk-for-go/services/privatedns/mgmt/2018-09-01/privatedns"
	"github.com/gardener/controller-manager-library/pkg/logger"

	"github.com/gardener/external-dns-management/pkg/controller/provider/azure/utils"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Change struct {
	rs   *azure.RecordSet
	Done provider.DoneHandler
}

type Execution struct {
	logger.LogContext
	handler       *Handler
	resourceGroup string
	zoneName      string

	changes map[string][]*Change
}

func NewExecution(logger logger.LogContext, h *Handler, resourceGroup string, zoneName string) *Execution {
	return &Execution{LogContext: logger, handler: h, resourceGroup: resourceGroup, zoneName: zoneName, changes: map[string][]*Change{}}
}

type buildStatus int

const (
	bs_ok                   buildStatus = 0
	bs_invalidType          buildStatus = 1
	bs_empty                buildStatus = 2
	bs_dryrun               buildStatus = 3
	bs_invalidName          buildStatus = 4
	bs_invalidRoutingPolicy buildStatus = 5
)

func (exec *Execution) buildRecordSet(req *provider.ChangeRequest) (buildStatus, azure.RecordType, *azure.RecordSet) {
	var dnsset *dns.DNSSet
	switch req.Action {
	case provider.R_CREATE, provider.R_UPDATE:
		dnsset = req.Addition
	case provider.R_DELETE:
		dnsset = req.Deletion
	}

	name, rset := dns.MapToProvider(req.Type, dnsset, exec.zoneName)
	name, ok := utils.DropZoneName(name, exec.zoneName)
	if !ok {
		return bs_invalidName, "", &azure.RecordSet{Name: &name}
	}

	if len(rset.Records) == 0 {
		return bs_empty, "", nil
	}

	if req.RoutingPolicy != nil {
		return bs_invalidRoutingPolicy, "", nil
	}

	exec.Infof("Desired %s: %s record set %s[%s] with TTL %d: %s", req.Action, rset.Type, name, exec.zoneName, rset.TTL, rset.RecordString())
	return exec.buildMappedRecordSet(name, rset)
}

func (exec *Execution) buildMappedRecordSet(name string, rset *dns.RecordSet) (buildStatus, azure.RecordType, *azure.RecordSet) {
	var properties azure.RecordSetProperties
	var recordType azure.RecordType

	properties.TTL = &rset.TTL
	switch rset.Type {
	case dns.RS_A:
		recordType = azure.A
		arecords := []azure.ARecord{}
		for _, r := range rset.Records {
			arecords = append(arecords, azure.ARecord{Ipv4Address: &r.Value})
		}
		properties.ARecords = &arecords
	case dns.RS_AAAA:
		recordType = azure.AAAA
		aaaarecords := []azure.AaaaRecord{}
		for _, r := range rset.Records {
			aaaarecords = append(aaaarecords, azure.AaaaRecord{Ipv6Address: &r.Value})
		}
		properties.AaaaRecords = &aaaarecords
	case dns.RS_CNAME:
		recordType = azure.CNAME
		properties.CnameRecord = &azure.CnameRecord{Cname: &rset.Records[0].Value}
	case dns.RS_TXT:
		recordType = azure.TXT
		txtrecords := []azure.TxtRecord{}
		for _, r := range rset.Records {
			// AzureDNS stores value as given, i.e. including quotes, so text value must be unquoted
			unquoted, err := strconv.Unquote(r.Value)
			if err != nil {
				unquoted = r.Value
			}
			txtrecords = append(txtrecords, azure.TxtRecord{Value: &[]string{unquoted}})
		}
		properties.TxtRecords = &txtrecords
	default:
		return bs_invalidType, "", nil
	}

	return bs_ok, recordType, &azure.RecordSet{Name: &name, RecordSetProperties: &properties}
}

func (exec *Execution) apply(action string, recordType azure.RecordType, rset *azure.RecordSet, metrics provider.Metrics) error {
	var err error
	switch action {
	case provider.R_CREATE, provider.R_UPDATE:
		err = exec.update(recordType, rset, metrics)
	case provider.R_DELETE:
		err = exec.delete(recordType, rset, metrics)
	}
	return err
}

func (exec *Execution) update(recordType azure.RecordType, rset *azure.RecordSet, metrics provider.Metrics) error {
	exec.handler.config.RateLimiter.Accept()
	_, err := exec.handler.recordsClient.CreateOrUpdate(exec.handler.ctx, exec.resourceGroup, exec.zoneName,
		recordType, *rset.Name, *rset, "", "")
	zoneID := utils.MakeZoneID(exec.resourceGroup, exec.zoneName)
	metrics.AddZoneRequests(zoneID, provider.M_UPDATERECORDS, 1)
	return err
}

func (exec *Execution) delete(recordType azure.RecordType, rset *azure.RecordSet, metrics provider.Metrics) error {
	exec.handler.config.RateLimiter.Accept()
	_, err := exec.handler.recordsClient.Delete(exec.handler.ctx, exec.resourceGroup, exec.zoneName, recordType, *rset.Name, "")
	zoneID := utils.MakeZoneID(exec.resourceGroup, exec.zoneName)
	metrics.AddZoneRequests(zoneID, provider.M_DELETERECORDS, 1)
	return err
}
