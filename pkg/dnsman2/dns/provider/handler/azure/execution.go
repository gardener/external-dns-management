// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package azure

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/go-logr/logr"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/azure/utils"
)

type execution struct {
	log           logr.Logger
	handler       *handler
	zone          provider.DNSHostedZone
	resourceGroup string
	zoneName      string
}

type recordSet struct {
	recordType armdns.RecordType
	recordSet  armdns.RecordSet
}

func newExecution(log logr.Logger, h *handler, zone provider.DNSHostedZone, resourceGroup, zoneName string) *execution {
	return &execution{log: log, handler: h, zone: zone, resourceGroup: resourceGroup, zoneName: zoneName}
}

func (exec *execution) apply(ctx context.Context, name dns.DNSSetName, req *provider.ChangeRequestUpdate) error {
	var (
		rsOld, rsNew *recordSet
		err          error
	)
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

	if rsOld != nil && rsNew == nil {
		return exec.delete(ctx, rsOld)
	} else if rsNew != nil {
		return exec.update(ctx, rsNew)
	}

	return fmt.Errorf("both old and new record sets are nil for %s", name)
}

func (exec *execution) buildRecordSet(name dns.DNSSetName, rs *dns.RecordSet) (*recordSet, error) {
	if rs.RoutingPolicy != nil || name.SetIdentifier != "" {
		return nil, fmt.Errorf("routing policies are not supported by the Azure DNS provider")
	}
	recordName, ok := utils.DropZoneName(name.DNSName, exec.zone.Domain())
	if !ok {
		return nil, fmt.Errorf("unexpected dns name %s", name.DNSName)
	}

	return exec.buildMappedRecordSet(recordName, rs)
}

func (exec *execution) buildMappedRecordSet(name string, rs *dns.RecordSet) (*recordSet, error) {
	var (
		properties = armdns.RecordSetProperties{TTL: &rs.TTL}
		recordType armdns.RecordType
	)
	switch rs.Type {
	case dns.TypeA:
		recordType = armdns.RecordTypeA
		var records []*armdns.ARecord
		for _, r := range rs.Records {
			records = append(records, &armdns.ARecord{IPv4Address: &r.Value})
		}
		properties.ARecords = records
	case dns.TypeAAAA:
		recordType = armdns.RecordTypeAAAA
		var records []*armdns.AaaaRecord
		for _, r := range rs.Records {
			records = append(records, &armdns.AaaaRecord{IPv6Address: &r.Value})
		}
		properties.AaaaRecords = records
	case dns.TypeCNAME:
		recordType = armdns.RecordTypeCNAME
		properties.CnameRecord = &armdns.CnameRecord{Cname: &rs.Records[0].Value}
	case dns.TypeTXT:
		recordType = armdns.RecordTypeTXT
		var records []*armdns.TxtRecord
		for _, r := range rs.Records {
			// Azure DNS stores value as given, i.e. including quotes, so text value must be unquoted
			unquoted, err := strconv.Unquote(r.Value)
			if err != nil {
				unquoted = r.Value
			}
			records = append(records, &armdns.TxtRecord{Value: []*string{&unquoted}})
		}
		properties.TxtRecords = records
	default:
		return nil, fmt.Errorf("record type %s not supported by Azure DNS provider", rs.Type)
	}
	return &recordSet{
		recordType: recordType,
		recordSet: armdns.RecordSet{
			Name:       &name,
			Properties: &properties,
		},
	}, nil
}

func (exec *execution) update(ctx context.Context, rs *recordSet) error {
	exec.logAction("update", rs)
	exec.handler.config.RateLimiter.Accept()
	zoneID := exec.zone.ZoneID().ID
	_, err := exec.handler.recordsClient.CreateOrUpdate(ctx, exec.resourceGroup, exec.zoneName, rs.Name(), rs.Type(), rs.Set(), nil)
	exec.handler.config.Metrics.AddZoneRequests(zoneID, provider.MetricsRequestTypeUpdateRecords, 1)
	return err
}

func (exec *execution) delete(ctx context.Context, rs *recordSet) error {
	exec.logAction("delete", rs)
	exec.handler.config.RateLimiter.Accept()
	zoneID := exec.zone.ZoneID().ID
	_, err := exec.handler.recordsClient.Delete(ctx, exec.resourceGroup, exec.zoneName, rs.Name(), rs.Type(), nil)
	exec.handler.config.Metrics.AddZoneRequests(zoneID, provider.MetricsRequestTypeDeleteRecords, 1)
	return err
}

func (exec *execution) logAction(action string, rs *recordSet) {
	exec.log.Info(fmt.Sprintf("Desired %s: %s record set %s[%s] with TTL %s: %s", action, rs.Type(), rs.Name(), exec.zone.Domain(), time.Duration(*rs.Set().Properties.TTL)*time.Second, rs.Records()))
}

func (rs *recordSet) Name() string {
	return *rs.recordSet.Name
}

func (rs *recordSet) Type() armdns.RecordType {
	return rs.recordType
}

func (rs *recordSet) Set() armdns.RecordSet {
	return rs.recordSet
}

func (rs *recordSet) Records() string {
	var records []string
	props := rs.Set().Properties
	switch rs.Type() {
	case armdns.RecordTypeA:
		for _, record := range props.ARecords {
			records = append(records, *record.IPv4Address)
		}
	case armdns.RecordTypeAAAA:
		for _, record := range props.AaaaRecords {
			records = append(records, *record.IPv6Address)
		}
	case armdns.RecordTypeCNAME:
		if props.CnameRecord != nil {
			records = append(records, *props.CnameRecord.Cname)
		}
	case armdns.RecordTypeTXT:
		for _, record := range props.TxtRecords {
			var values []string
			for _, value := range record.Value {
				values = append(values, *value)
			}
			records = append(records, strings.Join(values, "\n"))
		}
	}
	return strings.Join(records, ", ")
}
