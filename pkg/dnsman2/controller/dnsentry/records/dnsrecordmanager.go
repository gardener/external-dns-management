// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package records

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/providerselector"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

type DNSRecordManager struct {
	common.EntryContext
	State *state.State
}

type ZonedRequests = map[dns.ZoneID]map[dns.DNSSetName]*provider.ChangeRequests

func (m *DNSRecordManager) ApplyChangeRequests(providerData *providerselector.NewProviderData, zonedRequests ZonedRequests) *common.ReconcileResult {
	newZoneID := providerData.ZoneID
	for zoneID, perName := range zonedRequests {
		if zoneID == newZoneID {
			continue
		}
		providerAccount := providerData.ProviderState.GetAccount()
		if res := m.cleanupCrossZoneRecords(zoneID, perName, providerAccount); res != nil {
			return res
		}
	}

	changeRequestsPerName := zonedRequests[newZoneID]
	if len(changeRequestsPerName) > 0 {
		zones, err := providerData.ProviderState.GetAccount().GetZones(m.Ctx)
		if err != nil {
			m.Log.Error(err, "failed to get zones from DNS account", "provider", providerData.ProviderKey)
			return &common.ReconcileResult{Err: err}
		}
		var zone provider.DNSHostedZone
		for _, z := range zones {
			if z.ZoneID() == newZoneID {
				zone = z
				break
			}
		}
		if zone == nil {
			err := fmt.Errorf("zone %s not found in provider %s", newZoneID.ID, providerData.ProviderKey)
			m.Log.Error(err, "failed to find zone for DNS Entry")
			return &common.ReconcileResult{Err: err}
		}
		for _, changeRequests := range changeRequestsPerName {
			m.Log.V(1).Info("applying change requests", "zone", zone.ZoneID(), "requests", changeRequests)
			if err := providerData.ProviderState.GetAccount().ExecuteRequests(m.Ctx, zone, *changeRequests); err != nil {
				m.Log.Error(err, "failed to execute DNS change requests", "provider", providerData.ProviderKey)
				return &common.ReconcileResult{Err: err}
			}
		}
	}
	return nil
}

func (m *DNSRecordManager) QueryRecords(keys FullRecordKeySet) (map[FullRecordSetKey]*dns.RecordSet, *common.ReconcileResult) {
	zonesToCheck := sets.Set[dns.ZoneID]{}
	for key := range keys {
		zonesToCheck.Insert(key.ZoneID)
	}

	results := make(map[FullRecordSetKey]*dns.RecordSet)
	for zoneID := range zonesToCheck {
		queryHandler, err := m.State.GetDNSQueryHandler(zoneID)
		if err != nil {
			m.Log.Error(err, "failed to get DNS query handler for zone", "zoneID", zoneID.ID)
			return nil, &common.ReconcileResult{Err: fmt.Errorf("failed to get query handler for zone %s: %w", zoneID.ID, err)}
		}
		for key := range keys {
			if key.ZoneID != zoneID {
				continue
			}
			targets, policy, err := queryHandler.Query(m.Ctx, key.Name.DNSName, key.Name.SetIdentifier, key.RecordType)
			if err != nil {
				m.Log.Error(err, "failed to query DNS records", "name", key.Name, "type", key.RecordType, "zoneID", zoneID.ID)
				return nil, &common.ReconcileResult{Err: fmt.Errorf("failed to query DNS records for %s, type %s in zone %s: %w", key.Name, key.RecordType, zoneID.ID, err)}
			}
			if len(targets) > 0 {
				dnsSet := dns.NewDNSSet(key.Name)
				InsertRecordSets(*dnsSet, policy, targets)
				results[key] = dnsSet.Sets[key.RecordType]
			}
		}
	}
	return results, nil
}

func (m *DNSRecordManager) cleanupCrossZoneRecords(zoneID dns.ZoneID, perName map[dns.DNSSetName]*provider.ChangeRequests, account *provider.DNSAccount) *common.ReconcileResult {
	if len(perName) == 0 {
		return nil // Nothing to clean up
	}

	var zone *provider.DNSHostedZone
	zones, err := account.GetZones(m.Ctx)
	if err != nil {
		m.Log.Error(err, "failed to get zones from DNS account")
		return &common.ReconcileResult{Err: err}
	}
	for _, z := range zones {
		if z.ZoneID() == zoneID {
			zone = &z
			break
		}
	}
	if zone == nil {
		account, zone, err = m.State.FindAccountForZone(m.Ctx, zoneID) // Ensure the account is loaded for the zone
		if err != nil {
			m.Log.Error(err, "failed to find account for zone", "zoneID", zoneID)
			res := m.StatusUpdater().FailWithStatusError(fmt.Errorf("failed to find account for old zone %q to clean up old records", zoneID))
			return &res
		}
	}

	for name, changeRequests := range perName {
		if len(changeRequests.Updates) == 0 {
			continue // Nothing to delete for this name
		}
		m.Log.Info("deleting cross-zone records", "zoneID", zoneID, "name", name)
		m.Log.V(1).Info("deleting cross-zone records by applying change requests", "zone", zoneID, "requests", *changeRequests)
		if err := account.ExecuteRequests(m.Ctx, *zone, *changeRequests); err != nil {
			m.Log.Error(err, "failed to delete cross-zone records", "zoneID", zoneID, "name", name)
			return &common.ReconcileResult{Err: err}
		}
	}
	return nil
}
