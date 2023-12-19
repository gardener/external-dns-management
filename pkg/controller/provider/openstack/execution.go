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

package openstack

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gophercloud/gophercloud/openstack/dns/v2/recordsets"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Change struct {
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
	bsOk                   buildStatus = 0
	bsEmpty                buildStatus = 2
	bsDryRun               buildStatus = 3
	bsInvalidRoutingPolicy buildStatus = 4
)

func (exec *Execution) buildRecordSet(req *provider.ChangeRequest) (buildStatus, *recordsets.RecordSet) {
	var dnsset *dns.DNSSet
	switch req.Action {
	case provider.R_CREATE, provider.R_UPDATE:
		dnsset = req.Addition
	case provider.R_DELETE:
		dnsset = req.Deletion
	}

	if dnsset.RoutingPolicy != nil {
		return bsInvalidRoutingPolicy, nil
	}

	name, rset := dns.MapToProvider(req.Type, dnsset, exec.zone.Domain())

	if len(rset.Records) == 0 {
		return bsEmpty, nil
	}

	exec.Infof("Desired %s: %s record set %s[%s]: %s", req.Action, rset.Type, name, exec.zone.Domain(), rset.RecordString())
	return exec.buildMappedRecordSet(name.DNSName, rset)
}

func (exec *Execution) buildMappedRecordSet(name string, rset *dns.RecordSet) (buildStatus, *recordsets.RecordSet) {
	osRSet := recordsets.RecordSet{
		Name: name,
		TTL:  int(rset.TTL),
		Type: rset.Type,
	}

	for _, r := range rset.Records {
		value := r.Value
		if rset.Type == dns.RS_CNAME {
			value = dns.AlignHostname(value)
		}
		osRSet.Records = append(osRSet.Records, value)
	}

	return bsOk, &osRSet
}

func (exec *Execution) apply(action string, rset *recordsets.RecordSet) error {
	var err error
	switch action {
	case provider.R_CREATE:
		err = exec.create(rset)
	case provider.R_UPDATE:
		err = exec.update(rset)
	case provider.R_DELETE:
		err = exec.delete(rset)
	}
	return err
}

func (exec *Execution) create(rset *recordsets.RecordSet) error {
	opts := recordsets.CreateOpts{
		Name:    dns.AlignHostname(rset.Name),
		Type:    rset.Type,
		TTL:     rset.TTL,
		Records: rset.Records,
	}
	exec.handler.config.RateLimiter.Accept()
	_, err := exec.handler.client.CreateRecordSet(exec.zone.Id().ID, opts)
	return err
}

func (exec *Execution) lookupRecordSetID(rset *recordsets.RecordSet) (string, error) {
	name := dns.AlignHostname(rset.Name)
	recordSetID := ""
	handler := func(recordSet *recordsets.RecordSet) error {
		if recordSet.Name == name {
			recordSetID = recordSet.ID
		}
		return nil
	}
	exec.handler.config.RateLimiter.Accept()
	err := exec.handler.client.ForEachRecordSetFilterByTypeAndName(exec.zone.Id().ID, rset.Type, name, handler)
	if err != nil {
		return "", fmt.Errorf("RecordSet lookup for %s %s failed with: %s", rset.Type, rset.Name, err)
	}
	if recordSetID == "" {
		return "", fmt.Errorf("RecordSet %s %s not found for update", rset.Type, rset.Name)
	}
	return recordSetID, nil
}

func (exec *Execution) update(rset *recordsets.RecordSet) error {
	recordSetID, err := exec.lookupRecordSetID(rset)
	if err != nil {
		return err
	}

	opts := recordsets.UpdateOpts{
		TTL:     &rset.TTL,
		Records: rset.Records,
	}
	exec.handler.config.RateLimiter.Accept()
	err = exec.handler.client.UpdateRecordSet(exec.zone.Id().ID, recordSetID, opts)
	return err
}

func (exec *Execution) delete(rset *recordsets.RecordSet) error {
	recordSetID, err := exec.lookupRecordSetID(rset)
	if err != nil {
		return err
	}
	exec.handler.config.RateLimiter.Accept()
	err = exec.handler.client.DeleteRecordSet(exec.zone.Id().ID, recordSetID)
	return err
}
