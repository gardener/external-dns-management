// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package records

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/providerselector"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// DNSRecordManager manages DNS record changes and queries for DNS entries.
type DNSRecordManager struct {
	common.EntryContext
	State *state.State
}

// ZonedRequests maps a ZoneID to a map of DNSSetName and their corresponding ChangeRequests.
type ZonedRequests = map[dns.ZoneID]map[dns.DNSSetName]*provider.ChangeRequests

// ApplyChangeRequests applies the given change requests to the appropriate DNS zones and providers.
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
			return common.ErrorReconcileResult(fmt.Sprintf("failed to get zones for DNS account: %s", err), true)
		}
		var zone provider.DNSHostedZone
		for _, z := range zones {
			if z.ZoneID() == newZoneID {
				zone = z
				break
			}
		}
		if zone == nil {
			return common.ErrorReconcileResult(fmt.Sprintf("zone %s not found in provider %s", newZoneID.ID, providerData.ProviderKey), true)
		}
		for _, changeRequests := range changeRequestsPerName {
			m.Log.V(1).Info("applying change requests", "zone", zone.ZoneID(), "requests", changeRequests)
			if err := providerData.ProviderState.GetAccount().ExecuteRequests(m.Ctx, zone, *changeRequests); err != nil {
				return common.ErrorReconcileResult(fmt.Sprintf("failed to execute DNS change requests: %s", err), true)
			}
		}
	}
	return nil
}

// QueryRecords queries DNS records for the given set of record keys.
func (m *DNSRecordManager) QueryRecords(ctx context.Context, keys FullRecordKeySet) (map[FullRecordSetKey]*dns.RecordSet, *common.ReconcileResult) {
	zonesToCheck := sets.Set[dns.ZoneID]{}
	for key := range keys {
		zonesToCheck.Insert(key.ZoneID)
	}

	results := make(map[FullRecordSetKey]*dns.RecordSet)
	for zoneID := range zonesToCheck {
		queryHandler, err := m.State.GetDNSQueryHandler(ctx, zoneID)
		if err != nil {
			return nil, common.ErrorReconcileResult(fmt.Sprintf("failed to get DNS query handler for zone %s: %s", zoneID.ID, err), true)
		}
		for key := range keys {
			if key.ZoneID != zoneID {
				continue
			}
			targets, policy, err := queryHandler.Query(m.Ctx, key.Name, key.RecordType)
			if err != nil {
				m.Log.Error(err, "failed to query DNS records", "name", key.Name, "type", key.RecordType, "zoneID", zoneID.ID)
				return nil, common.ErrorReconcileResult(fmt.Sprintf("failed to query DNS records for %s, type %s in zone %s: %s", key.Name, key.RecordType, zoneID.ID, err), true)
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

// cleanupCrossZoneRecords cleans up DNS records that exist in zones other than the current one.
func (m *DNSRecordManager) cleanupCrossZoneRecords(zoneID dns.ZoneID, perName map[dns.DNSSetName]*provider.ChangeRequests, account *provider.DNSAccount) *common.ReconcileResult {
	if len(perName) == 0 {
		return nil // Nothing to clean up
	}

	var zone *provider.DNSHostedZone
	zones, err := account.GetZones(m.Ctx)
	if err != nil {
		return common.ErrorReconcileResult(fmt.Sprintf("failed to get zones for DNS account: %s", err), true)
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
			return common.ErrorReconcileResult(fmt.Sprintf("failed to find account for zone %s: %s", zoneID, err), true)
		}
	}

	for name, changeRequests := range perName {
		if len(changeRequests.Updates) == 0 {
			continue // Nothing to delete for this name
		}
		m.Log.Info("deleting cross-zone records", "zoneID", zoneID, "name", name)
		m.Log.V(1).Info("deleting cross-zone records by applying change requests", "zone", zoneID, "requests", *changeRequests)
		if err := account.ExecuteRequests(m.Ctx, *zone, *changeRequests); err != nil {
			return common.ErrorReconcileResult(fmt.Sprintf("failed to delete cross-zone records for %s[%s]: %s", name, zoneID, err), true)
		}
	}
	return nil
}
