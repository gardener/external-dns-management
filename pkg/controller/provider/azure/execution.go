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

package azure

import (
	"strings"

	azure "github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-03-01-preview/dns"
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

func (exec *Execution) buildRecordSet(req *provider.ChangeRequest) (buildStatus, azure.RecordType, *azure.RecordSet) {
	var dnsset *dns.DNSSet
	switch req.Action {
	case provider.R_CREATE, provider.R_UPDATE:
		dnsset = req.Addition
	case provider.R_DELETE:
		dnsset = req.Deletion
	}

	name, rset := dns.MapToProvider(req.Type, dnsset)
	name, ok := dropZoneName(name, exec.zoneName)
	if !ok {
		return bs_invalidName, "", &azure.RecordSet{Name: &name}
	}

	if len(rset.Records) == 0 {
		return bs_empty, "", nil
	}

	exec.Infof("Desired %s: %s record set %s[%s]: %s", req.Action, rset.Type, name, exec.zoneName, rset.RecordString())
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
	case dns.RS_CNAME:
		recordType = azure.CNAME
		properties.CnameRecord = &azure.CnameRecord{Cname: &rset.Records[0].Value}
	case dns.RS_TXT:
		recordType = azure.TXT
		txtrecords := []azure.TxtRecord{}
		for _, r := range rset.Records {
			txtrecords = append(txtrecords, azure.TxtRecord{Value: &[]string{r.Value}})
		}
		properties.TxtRecords = &txtrecords
	default:
		return bs_invalidType, "", nil
	}

	return bs_ok, recordType, &azure.RecordSet{Name: &name, RecordSetProperties: &properties}
}

func (exec *Execution) apply(action string, recordType azure.RecordType, rset *azure.RecordSet) error {
	var err error
	switch action {
	case provider.R_CREATE, provider.R_UPDATE:
		err = exec.update(recordType, rset)
	case provider.R_DELETE:
		err = exec.delete(recordType, rset)
	}
	return err
}

func (exec *Execution) update(recordType azure.RecordType, rset *azure.RecordSet) error {
	_, err := exec.handler.recordsClient.CreateOrUpdate(exec.handler.ctx, exec.resourceGroup, exec.zoneName, *rset.Name,
		recordType, *rset, "", "")
	return err
}

func (exec *Execution) delete(recordType azure.RecordType, rset *azure.RecordSet) error {
	_, err := exec.handler.recordsClient.Delete(exec.handler.ctx, exec.resourceGroup, exec.zoneName, *rset.Name, recordType, "")
	return err
}
