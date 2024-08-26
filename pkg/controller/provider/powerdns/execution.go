// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package powerdns

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/joeig/go-powerdns/v3"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type RecordSet struct {
	Name       string
	RecordType powerdns.RRType
	TTL        uint32
	Content    []string
}

type Execution struct {
	logger.LogContext
	handler *Handler
	zone    provider.DNSHostedZone
}

func NewExecution(logger logger.LogContext, h *Handler, zone provider.DNSHostedZone) *Execution {
	return &Execution{LogContext: logger, handler: h, zone: zone}
}

func (exec *Execution) buildRecordSet(req *provider.ChangeRequest) (*RecordSet, error) {
	var dnsset *dns.DNSSet

	switch req.Action {
	case provider.R_CREATE, provider.R_UPDATE:
		dnsset = req.Addition
	case provider.R_DELETE:
		dnsset = req.Deletion
	}

	name, rset := dns.MapToProvider(req.Type, dnsset, exec.zone.Domain())

	if name.SetIdentifier != "" || dnsset.RoutingPolicy != nil {
		return nil, fmt.Errorf("routing policies not supported for " + TYPE_CODE)
	}

	if name.DNSName == "" || len(rset.Records) == 0 {
		return nil, nil
	}

	exec.Infof("Desired %s: %s record set %s[%s] with TTL %d: %s", req.Action, rset.Type, name.DNSName, exec.zone.Id(), rset.TTL, rset.RecordString())

	recordSet := RecordSet{
		Name:       name.DNSName,
		RecordType: powerdns.RRType(rset.Type),
	}

	switch req.Action {
	case provider.R_CREATE, provider.R_UPDATE:
		var content []string
		for _, record := range rset.Records {
			content = append(content, record.Value)
		}

		recordSet.Content = content
		recordSet.TTL = uint32(rset.TTL)
	}

	return &recordSet, nil
}

func (exec *Execution) apply(action string, rset *RecordSet, metrics provider.Metrics) error {
	var err error
	switch action {
	case provider.R_CREATE, provider.R_UPDATE:
		err = exec.update(rset, metrics)
	case provider.R_DELETE:
		err = exec.delete(rset, metrics)
	}
	return err
}

func (exec *Execution) update(rset *RecordSet, metrics provider.Metrics) error {
	exec.handler.config.RateLimiter.Accept()
	zoneID := exec.zone.Id().ID
	err := exec.handler.powerdns.Records.Change(exec.handler.ctx, zoneID, rset.Name, rset.RecordType, rset.TTL, rset.Content)
	metrics.AddZoneRequests(zoneID, provider.M_UPDATERECORDS, 1)
	return err
}

func (exec *Execution) delete(rset *RecordSet, metrics provider.Metrics) error {
	exec.handler.config.RateLimiter.Accept()
	zoneID := exec.zone.Id().ID
	err := exec.handler.powerdns.Records.Delete(exec.handler.ctx, zoneID, rset.Name, rset.RecordType)
	metrics.AddZoneRequests(zoneID, provider.M_DELETERECORDS, 1)
	return err
}
