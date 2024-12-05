// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package azure

import (
	"strconv"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/controller/provider/azure/utils"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Change struct {
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

func (exec *Execution) buildRecordSet(req *provider.ChangeRequest) (buildStatus, armdns.RecordType, *armdns.RecordSet) {
	var dnsset *dns.DNSSet
	switch req.Action {
	case provider.R_CREATE, provider.R_UPDATE:
		dnsset = req.Addition
	case provider.R_DELETE:
		dnsset = req.Deletion
	}

	if dnsset.RoutingPolicy != nil {
		return bs_invalidRoutingPolicy, "", nil
	}

	name, ok := utils.DropZoneName(dnsset.Name.DNSName, exec.zoneName)
	if !ok {
		return bs_invalidName, "", &armdns.RecordSet{Name: &name}
	}

	rset := dnsset.Sets[req.Type]
	if len(rset.Records) == 0 {
		return bs_empty, "", nil
	}

	exec.Infof("Desired %s: %s record set %s[%s] with TTL %d: %s", req.Action, rset.Type, name, exec.zoneName, rset.TTL, rset.RecordString())
	return exec.buildMappedRecordSet(name, rset)
}

func (exec *Execution) buildMappedRecordSet(name string, rset *dns.RecordSet) (buildStatus, armdns.RecordType, *armdns.RecordSet) {
	var properties armdns.RecordSetProperties
	var recordType armdns.RecordType

	properties.TTL = &rset.TTL
	switch rset.Type {
	case dns.RS_A:
		recordType = armdns.RecordTypeA
		arecords := []*armdns.ARecord{}
		for _, r := range rset.Records {
			arecords = append(arecords, &armdns.ARecord{IPv4Address: &r.Value})
		}
		properties.ARecords = arecords
	case dns.RS_AAAA:
		recordType = armdns.RecordTypeAAAA
		aaaarecords := []*armdns.AaaaRecord{}
		for _, r := range rset.Records {
			aaaarecords = append(aaaarecords, &armdns.AaaaRecord{IPv6Address: &r.Value})
		}
		properties.AaaaRecords = aaaarecords
	case dns.RS_CNAME:
		recordType = armdns.RecordTypeCNAME
		properties.CnameRecord = &armdns.CnameRecord{Cname: &rset.Records[0].Value}
	case dns.RS_TXT:
		recordType = armdns.RecordTypeTXT
		txtrecords := []*armdns.TxtRecord{}
		for _, r := range rset.Records {
			// AzureDNS stores value as given, i.e. including quotes, so text value must be unquoted
			unquoted, err := strconv.Unquote(r.Value)
			if err != nil {
				unquoted = r.Value
			}
			txtrecords = append(txtrecords, &armdns.TxtRecord{Value: []*string{&unquoted}})
		}
		properties.TxtRecords = txtrecords
	default:
		return bs_invalidType, "", nil
	}

	return bs_ok, recordType, &armdns.RecordSet{Name: &name, Properties: &properties}
}

func (exec *Execution) apply(action string, recordType armdns.RecordType, rset *armdns.RecordSet, metrics provider.Metrics) error {
	var err error
	switch action {
	case provider.R_CREATE, provider.R_UPDATE:
		err = exec.update(recordType, rset, metrics)
	case provider.R_DELETE:
		err = exec.delete(recordType, rset, metrics)
	}
	return err
}

func (exec *Execution) update(recordType armdns.RecordType, rset *armdns.RecordSet, metrics provider.Metrics) error {
	exec.handler.config.RateLimiter.Accept()
	_, err := exec.handler.recordsClient.CreateOrUpdate(exec.handler.ctx, exec.resourceGroup, exec.zoneName, *rset.Name,
		recordType, *rset, nil)
	zoneID := utils.MakeZoneID(exec.resourceGroup, exec.zoneName)
	metrics.AddZoneRequests(zoneID, provider.M_UPDATERECORDS, 1)
	return err
}

func (exec *Execution) delete(recordType armdns.RecordType, rset *armdns.RecordSet, metrics provider.Metrics) error {
	exec.handler.config.RateLimiter.Accept()
	_, err := exec.handler.recordsClient.Delete(exec.handler.ctx, exec.resourceGroup, exec.zoneName, *rset.Name, recordType, nil)
	zoneID := utils.MakeZoneID(exec.resourceGroup, exec.zoneName)
	metrics.AddZoneRequests(zoneID, provider.M_DELETERECORDS, 1)
	return err
}
