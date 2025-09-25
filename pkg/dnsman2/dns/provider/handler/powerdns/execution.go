// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package powerdns

import (
	"context"
	"fmt"
	"strconv"

	"github.com/joeig/go-powerdns/v3"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type recordSet struct {
	Name       string
	RecordType powerdns.RRType
	TTL        uint32
	Content    []string
}

type execution struct {
	handler *handler
	zone    provider.DNSHostedZone
}

func newExecution(h *handler, zone provider.DNSHostedZone) *execution {
	return &execution{handler: h, zone: zone}
}

func (exec *execution) apply(ctx context.Context, name dns.DNSSetName, req *provider.ChangeRequestUpdate) error {
	var rsOld, rsNew *recordSet
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
	log, err := exec.handler.getLogFromContext(ctx)
	if err != nil {
		return err
	}

	switch {
	case rsNew != nil:
		log.Info(fmt.Sprintf("Desired %s: %s record set %s[%s] with TTL %d: %s", "UPSERT", rsNew.RecordType, name.DNSName, exec.zone.ZoneID().ID, rsNew.TTL, req.New.RecordString()))
		return exec.update(ctx, rsNew)
	case rsOld != nil && rsNew == nil:
		log.Info(fmt.Sprintf("Desired %s: %s record set %s[%s] with TTL %d: %s", "DELETE", rsOld.RecordType, name.DNSName, exec.zone.ZoneID().ID, rsOld.TTL, req.Old.RecordString()))
		return exec.delete(ctx, rsOld)
	}
	return fmt.Errorf("both old and new record sets are nil for %s", name)
}

func (exec *execution) buildRecordSet(name dns.DNSSetName, rs *dns.RecordSet) (*recordSet, error) {
	if rs.RoutingPolicy != nil || name.SetIdentifier != "" {
		return nil, fmt.Errorf("PowerDNS provider does not support routing policies")
	}

	internalRS := &recordSet{
		Name:       name.DNSName,
		TTL:        utils.TTLToUint32(rs.TTL),
		RecordType: powerdns.RRType(rs.Type),
	}

	for _, r := range rs.Records {
		value := r.Value
		switch rs.Type {
		case dns.TypeCNAME:
			value = dns.EnsureTrailingDot(value)
		case dns.TypeTXT:
			value = strconv.Quote(value)
		}
		internalRS.Content = append(internalRS.Content, value)
	}

	return internalRS, nil
}

func (exec *execution) update(ctx context.Context, rset *recordSet) error {
	exec.handler.config.RateLimiter.Accept()
	zoneID := exec.zone.ZoneID().ID
	err := exec.handler.powerdns.Records.Change(ctx, zoneID, rset.Name, rset.RecordType, rset.TTL, rset.Content)
	exec.handler.config.Metrics.AddZoneRequests(zoneID, provider.MetricsRequestTypeUpdateRecords, 1)
	return err
}

func (exec *execution) delete(ctx context.Context, rset *recordSet) error {
	exec.handler.config.RateLimiter.Accept()
	zoneID := exec.zone.ZoneID().ID
	err := exec.handler.powerdns.Records.Delete(ctx, zoneID, rset.Name, rset.RecordType)
	exec.handler.config.Metrics.AddZoneRequests(zoneID, provider.MetricsRequestTypeDeleteRecords, 1)
	return err
}
