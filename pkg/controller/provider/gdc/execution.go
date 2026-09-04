// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package gdc

import (
	"context"
	"fmt"
	"slices"

	"github.com/gardener/controller-manager-library/pkg/logger"
	globalnetworkingv1 "github.com/googlecloudplatform/google-distributed-cloud-apis/pkg/apis/public/global/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dnsutil "github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/dns"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Change struct {
	RecordSet           *resourceRecordSet
	RecordType          string // supported record type: A, TXT
	DnsZone             string // Name of the DNS zone the record will be in
	ChangeType          string // supported change type: create, update, delete
	ObjectMetaName      string // ResourceRecordSet Name
	ObjectMetaNamespace string // ResourceRecordSet Namespace
	SetIdentifier       string // (Optional) Identifier for the RecordSet
}

type Execution struct {
	logger.LogContext
	handler *Handler
	zone    provider.DNSHostedZone
}

func NewExecution(log logger.LogContext, h *Handler, zone provider.DNSHostedZone) *Execution {
	return &Execution{
		LogContext: log,
		handler:    h,
		zone:       zone,
	}
}

func (exec *Execution) prepareChange(req *provider.ChangeRequest) (*Change, error) {
	var dnsset *dns.DNSSet
	switch req.Action {
	case provider.R_CREATE, provider.R_UPDATE:
		dnsset = req.Addition
	case provider.R_DELETE:
		dnsset = req.Deletion
	}

	// MapToProvider func will also convert META to TXT.
	newSet := dnsset.Sets[req.Type]
	if !slices.Contains(supportedRecordTypes, newSet.Type) {
		return nil, fmt.Errorf("unsupported record type %s", req.Type)
	}

	objectMetaName := dnsutil.GetDNSRecordSetName(newSet.Type, dnsset.Name.DNSName)

	exec.Infof("%s %s record set %s[%s]: %s(%d)", req.Action, newSet.Type, dnsset.Name.DNSName, exec.zone.Id(), newSet.RecordString(), newSet.TTL)

	rrSet := mapRecordSet(dnsset.Name.DNSName, newSet)
	return &Change{
		RecordSet:           rrSet,
		RecordType:          newSet.Type,
		DnsZone:             exec.zone.Key(),
		ChangeType:          req.Action,
		ObjectMetaName:      objectMetaName,
		ObjectMetaNamespace: exec.handler.project,
		SetIdentifier:       dnsset.Name.SetIdentifier,
	}, nil
}

func resourceRecordSetForChange(change *Change) *globalnetworkingv1.ResourceRecordSet {
	recordSet := &globalnetworkingv1.ResourceRecordSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      change.ObjectMetaName,
			Namespace: change.ObjectMetaNamespace,
			Annotations: map[string]string{
				// We need to make sure to preserve the original FQDN (before changes due to
				// k8s-object-name limitations) since this will be used at a later point to match
				// records retrieved from a hosted zone with records known to a running instance
				// of the controller in the seed
				dnsutil.OriginalFQDNAnnotationKey: change.RecordSet.fqdn,
			},
		},
	}
	if change.SetIdentifier != "" {
		// Similar to above, the set identifier (if it exists) is used when matching records
		// retrieved from the infra to those known to the controller in the case of
		// routing policies.
		recordSet.Annotations[dnsutil.SetIdentifierKey] = change.SetIdentifier
	}
	if change.ChangeType == provider.R_CREATE || change.ChangeType == provider.R_UPDATE {
		ttl := uint32(change.RecordSet.ttl) // #nosec G115 -- TTL is safe bounded integer
		recordSet.Spec = globalnetworkingv1.ResourceRecordSetSpec{
			Name:       change.RecordSet.fqdn,
			TTLSeconds: &ttl,
			Type:       change.RecordType,
			RRData:     change.RecordSet.data,
			DNSZone:    change.DnsZone,
		}
	}

	return recordSet
}

func (exec *Execution) applyChange(change *Change) error {
	if len(change.RecordSet.data) == 0 {
		return nil
	}

	exec.Infof("processing changes for zone %s", exec.zone.Id())
	exec.handler.config.Metrics.AddZoneRequests(exec.zone.Id().ID, provider.M_UPDATERECORDS, 1)
	exec.handler.config.RateLimiter.Accept()

	recordSet := resourceRecordSetForChange(change)

	if change.ChangeType == provider.R_CREATE || change.ChangeType == provider.R_UPDATE {
		_, err := controllerutil.CreateOrUpdate(context.Background(), exec.handler.client, recordSet, func() error {
			return nil
		})
		if err != nil {
			return err
		}
	}
	if change.ChangeType == provider.R_DELETE {
		err := exec.handler.client.Delete(context.Background(), recordSet)
		if err != nil {
			return err
		}
	}
	return nil
}

// mapRecordSet maps dns.RecordSet to recordsets.ResourceRecordSet of gdc
func mapRecordSet(dnsName string, rs *dns.RecordSet) *resourceRecordSet {
	rrSetData := []string{}
	for _, record := range rs.Records {
		rrSetData = append(rrSetData, record.Value)
	}

	ttl := int(rs.TTL)
	if ttl <= 0 {
		ttl = dnsutil.DNSDefaultTTL
	}

	rrset := &resourceRecordSet{
		fqdn: dnsName,
		ttl:  ttl,
		data: rrSetData,
	}

	return rrset
}

type resourceRecordSet struct {
	data []string
	fqdn string
	ttl  int
}
