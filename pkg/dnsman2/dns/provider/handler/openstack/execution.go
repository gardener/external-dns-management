// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

type execution struct {
	log     logr.Logger
	handler *handler
	zone    provider.DNSHostedZone
}

func newExecution(log logr.Logger, h *handler, zone provider.DNSHostedZone) *execution {
	return &execution{log: log, handler: h, zone: zone}
}

func (exec *execution) apply(ctx context.Context, name dns.DNSSetName, req *provider.ChangeRequestUpdate) error {
	var rsOld, rsNew *recordsets.RecordSet
	var err error
	if req.Old != nil {
		rsOld, err = exec.buildRecordSet(name, req.Old)
		if err != nil {
			return err
		}
	}
	if req.New != nil {
		rsNew, err = exec.buildRecordSet(name, req.New)
		if err != nil {
			return err
		}
	}
	switch {
	case rsOld == nil && rsNew != nil:
		return exec.create(ctx, rsNew)
	case rsOld != nil && rsNew != nil:
		return exec.update(ctx, rsNew)
	case rsOld != nil && rsNew == nil:
		return exec.delete(ctx, rsOld)
	}
	return fmt.Errorf("both old and new record sets are nil for %s", name)
}

func (exec *execution) buildRecordSet(name dns.DNSSetName, rs *dns.RecordSet) (*recordsets.RecordSet, error) {
	if rs.RoutingPolicy != nil || name.SetIdentifier != "" {
		return nil, fmt.Errorf("OpenStack provider does not support routing policies")
	}

	osRSet := &recordsets.RecordSet{
		Name: name.DNSName,
		TTL:  int(rs.TTL),
		Type: string(rs.Type),
	}

	for _, r := range rs.Records {
		value := r.Value
		switch rs.Type {
		case dns.TypeCNAME:
			value = dns.EnsureTrailingDot(value)
		case dns.TypeTXT:
			value = strconv.Quote(value)
		}
		osRSet.Records = append(osRSet.Records, value)
	}

	return osRSet, nil
}

func (exec *execution) create(ctx context.Context, rs *recordsets.RecordSet) error {
	exec.logAction(rs.Name, "create", rs)

	opts := recordsets.CreateOpts{
		Name:    dns.EnsureTrailingDot(rs.Name),
		Type:    rs.Type,
		TTL:     rs.TTL,
		Records: rs.Records,
	}
	exec.handler.config.RateLimiter.Accept()
	_, err := exec.handler.client.CreateRecordSet(ctx, exec.zone.ZoneID().ID, opts)
	return err
}

func (exec *execution) lookupRecordSetID(ctx context.Context, rset *recordsets.RecordSet) (string, error) {
	name := dns.EnsureTrailingDot(rset.Name)
	recordSetID := ""
	handler := func(recordSet *recordsets.RecordSet) error {
		if recordSet.Name == name {
			recordSetID = recordSet.ID
		}
		return nil
	}
	exec.handler.config.RateLimiter.Accept()
	err := exec.handler.client.ForEachRecordSetFilterByTypeAndName(ctx, exec.zone.ZoneID().ID, rset.Type, name, handler)
	if err != nil {
		return "", fmt.Errorf("RecordSet lookup for %s %s failed with: %s", rset.Type, rset.Name, err)
	}
	if recordSetID == "" {
		return "", fmt.Errorf("RecordSet %s %s not found for update", rset.Type, rset.Name)
	}
	return recordSetID, nil
}

func (exec *execution) update(ctx context.Context, rs *recordsets.RecordSet) error {
	exec.logAction(rs.Name, "update", rs)

	recordSetID, err := exec.lookupRecordSetID(ctx, rs)
	if err != nil {
		return err
	}

	opts := recordsets.UpdateOpts{
		TTL:     &rs.TTL,
		Records: rs.Records,
	}
	exec.handler.config.RateLimiter.Accept()
	err = exec.handler.client.UpdateRecordSet(ctx, exec.zone.ZoneID().ID, recordSetID, opts)
	return err
}

func (exec *execution) delete(ctx context.Context, rs *recordsets.RecordSet) error {
	exec.logAction(rs.Name, "delete", rs)

	recordSetID, err := exec.lookupRecordSetID(ctx, rs)
	if err != nil {
		return err
	}
	exec.handler.config.RateLimiter.Accept()
	err = exec.handler.client.DeleteRecordSet(ctx, exec.zone.ZoneID().ID, recordSetID)
	return err
}

func (exec *execution) logAction(name, action string, rs *recordsets.RecordSet) {
	exec.log.Info(fmt.Sprintf("Desired %s: %s record set %s[%s]: [%s]", action, rs.Type, name, exec.zone.Domain(), strings.Join(rs.Records, ", ")))
}
