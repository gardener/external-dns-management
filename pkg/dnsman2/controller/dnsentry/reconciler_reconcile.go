// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"fmt"
	"strings"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/lookup"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/providerselector"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/records"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/targets"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

type entryReconciliation struct {
	common.EntryContext
	namespace                  string
	class                      string
	state                      *state.State
	lookupProcessor            lookup.LookupProcessor
	defaultCNAMELookupInterval int64
}

type newTargetsData struct {
	providerKey   client.ObjectKey
	providerType  string
	zoneID        dns.ZoneID
	dnsSet        *dns.DNSSet
	targets       dns.Targets
	routingPolicy *dns.RoutingPolicy
	warnings      []string
}

type zonedRequests = map[dns.ZoneID]map[dns.DNSSetName]*provider.ChangeRequests

func (r *entryReconciliation) reconcile() common.ReconcileResult {
	if res := r.ignoredByAnnotation(); res != nil {
		return *res
	}

	newProviderData, res := providerselector.CalcNewProvider(r.EntryContext, r.namespace, r.class, r.state)
	if res != nil {
		return *res
	}
	if newProviderData == nil {
		return r.updateStatusWithoutProvider()
	}
	if res := r.checkProviderNotReady(newProviderData); res != nil {
		return *res
	}

	recordsToCheck := records.FullRecordKeySet{}
	if res := r.calcOldTargets(recordsToCheck); res != nil {
		return *res
	}

	targetsData, res := r.calcNewTargets(newProviderData, recordsToCheck)
	if res != nil {
		return *res
	}

	actualRecords, res := r.dnsRecordManager().QueryRecords(recordsToCheck)
	if res != nil {
		return *res
	}

	var doneHandler provider.DoneHandler // TODO(MartinWeindel): Implement a done handler if needed
	zonedRequests := calculateZonedChangeRequests(targetsData, doneHandler, actualRecords)

	if res := r.dnsRecordManager().ApplyChangeRequests(newProviderData, zonedRequests); res != nil {
		return *res
	}

	return r.updateStatusWithProvider(targetsData)
}

func (r *entryReconciliation) updateStatusWithoutProvider() common.ReconcileResult {
	return r.StatusUpdater().UpdateStatus(func(status *v1alpha1.DNSEntryStatus) error {
		status.Provider = nil
		status.ObservedGeneration = r.Entry.Generation
		if len(status.Targets) > 0 && status.Zone != nil {
			status.State = v1alpha1.StateStale
		} else {
			status.State = v1alpha1.StateError
			status.ProviderType = nil
		}
		status.Message = ptr.To("no matching DNS provider found")
		return nil
	})
}

func (r *entryReconciliation) updateStatusWithProvider(targetsData *newTargetsData) common.ReconcileResult {
	return r.StatusUpdater().UpdateStatus(func(status *v1alpha1.DNSEntryStatus) error {
		status.Provider = &targetsData.providerKey.Name
		status.ProviderType = ptr.To(targetsData.providerType)
		status.Zone = &targetsData.zoneID.ID
		name := targetsData.dnsSet.Name
		rp := targetsData.routingPolicy
		status.DNSName = ptr.To(name.DNSName)
		if name.SetIdentifier != "" && rp != nil {
			status.RoutingPolicy = &v1alpha1.RoutingPolicy{
				Type:          string(rp.Type),
				Parameters:    rp.Parameters,
				SetIdentifier: name.SetIdentifier,
			}
		} else {
			status.RoutingPolicy = nil
		}
		status.Targets = targets.TargetsToStrings(targetsData.targets)
		if len(targetsData.targets) > 0 {
			status.TTL = ptr.To(targetsData.targets[0].GetTTL())
		} else {
			status.TTL = nil
		}

		status.ObservedGeneration = r.Entry.Generation
		status.State = v1alpha1.StateReady
		if len(targetsData.warnings) > 0 {
			status.Message = ptr.To(fmt.Sprintf("reconciled with warnings: %s", strings.Join(targetsData.warnings, ", ")))
		} else {
			status.Message = ptr.To("dns Entry active")
		}
		return nil
	})
}

func (r *entryReconciliation) ignoredByAnnotation() *common.ReconcileResult {
	if annotation, b := ignoreByAnnotation(r.Entry); b {
		r.Log.V(1).Info("Ignoring Entry due to annotation", "annotation", annotation)
		if r.Entry.DeletionTimestamp != nil {
			if res := r.StatusUpdater().RemoveFinalizer(); res != nil {
				return res
			}
		}
		return &common.ReconcileResult{}
	}
	return nil
}

func (r *entryReconciliation) calcOldTargets(recordsToCheck records.FullRecordKeySet) *common.ReconcileResult {
	status := r.Entry.Status
	oldTargets, err := targets.StatusToTargets(&status, r.Entry.Annotations[dns.AnnotationIPStack])
	if err != nil {
		// should not happen, but if it does, we want to log it
		return &common.ReconcileResult{Err: err}
	}
	if status.ProviderType != nil && status.Zone != nil && status.DNSName != nil {
		name, _, err := toDNSSetName(*status.DNSName, status.RoutingPolicy)
		if err != nil {
			return &common.ReconcileResult{Err: err}
		}
		oldZoneID := &dns.ZoneID{ID: *status.Zone, ProviderType: *status.ProviderType}
		// targets from status are already mapped to the provider, so we can use them directly
		records.InsertRecordKeys(recordsToCheck, records.FullDNSSetName{ZoneID: *oldZoneID, Name: name}, oldTargets)
	}
	return nil
}

func (r *entryReconciliation) checkProviderNotReady(data *providerselector.NewProviderData) *common.ReconcileResult {
	if data.Provider.Status.State != v1alpha1.StateReady {
		var err error
		if data.Provider.Status.State != "" {
			err = fmt.Errorf("provider %s has status %s: %s", data.ProviderKey, data.Provider.Status.State, ptr.Deref(data.Provider.Status.Message, "unknown error"))
		} else {
			err = fmt.Errorf("provider %s is not ready yet", data.ProviderKey)
		}
		res := r.StatusUpdater().FailWithStatusStale(err)
		return &res
	}
	return nil
}

func (r *entryReconciliation) calcNewTargets(providerData *providerselector.NewProviderData, recordsToCheck records.FullRecordKeySet) (*newTargetsData, *common.ReconcileResult) {
	producer := targets.NewTargetsProducer(r.Ctx, providerData.DefaultTTL, r.defaultCNAMELookupInterval, r.lookupProcessor)

	result, err := producer.FromSpec(client.ObjectKeyFromObject(r.Entry), &r.Entry.Spec, r.Entry.Annotations[dns.AnnotationIPStack])
	if err != nil {
		return nil, r.StatusUpdater().UpdateStatusInvalid(err)
	}
	name, rp, err := toDNSSetName(r.Entry.Spec.DNSName, r.Entry.Spec.RoutingPolicy)
	if err != nil {
		return nil, &common.ReconcileResult{Err: err}
	}

	account := providerData.ProviderState.GetAccount()
	if account == nil {
		res := r.StatusUpdater().FailWithStatusError(fmt.Errorf("provider %s is not ready yet", providerData.ProviderKey))
		return nil, &res
	}
	newTargets := account.MapTargets(r.Entry.Spec.DNSName, result.Targets)
	newDNSSet := dns.NewDNSSet(name)
	newZoneID := providerData.ZoneID
	records.InsertRecordKeys(recordsToCheck, records.FullDNSSetName{ZoneID: newZoneID, Name: name}, newTargets)
	if r.Entry.DeletionTimestamp == nil {
		records.InsertRecordSets(*newDNSSet, rp, newTargets)
	} else {
		newTargets = nil
	}
	return &newTargetsData{
		providerKey:   providerData.ProviderKey,
		providerType:  providerData.Provider.Spec.Type,
		zoneID:        newZoneID,
		dnsSet:        newDNSSet,
		targets:       newTargets,
		routingPolicy: rp,
		warnings:      result.Warnings,
	}, nil
}

func (r *entryReconciliation) dnsRecordManager() *records.DNSRecordManager {
	return &records.DNSRecordManager{
		EntryContext: r.EntryContext,
		State:        r.state,
	}
}

type updatePair struct {
	old *dns.RecordSet
	new *dns.RecordSet
}
type changeSet struct {
	additions map[records.FullRecordSetKey]*dns.RecordSet
	updates   map[records.FullRecordSetKey]*updatePair
	deletions map[records.FullRecordSetKey]*dns.RecordSet
}

func calculateZonedChangeRequests(targetsData *newTargetsData, doneHandler provider.DoneHandler, actual map[records.FullRecordSetKey]*dns.RecordSet) zonedRequests {
	changeSet := changeSet{
		additions: make(map[records.FullRecordSetKey]*dns.RecordSet),
		updates:   make(map[records.FullRecordSetKey]*updatePair),
		deletions: make(map[records.FullRecordSetKey]*dns.RecordSet),
	}

	newDNSSet := targetsData.dnsSet
	newFullDNSSetName := records.FullDNSSetName{Name: newDNSSet.Name, ZoneID: targetsData.zoneID}
	for actualKey, actualRecordSet := range actual {
		if actualKey.FullDNSSetName != newFullDNSSetName || newDNSSet.Sets[actualKey.RecordType] == nil {
			changeSet.deletions[actualKey] = actualRecordSet
		} else if !newDNSSet.Sets[actualKey.RecordType].Match(actualRecordSet) {
			changeSet.updates[actualKey] = &updatePair{
				old: actualRecordSet,
				new: newDNSSet.Sets[actualKey.RecordType],
			}
		}
	}
	for newRecordType, newRecordSet := range newDNSSet.Sets {
		newKey := records.FullRecordSetKey{
			FullDNSSetName: newFullDNSSetName,
			RecordType:     newRecordType,
		}
		if _, exists := actual[newKey]; !exists {
			changeSet.additions[newKey] = newRecordSet
		}
	}

	return convertChangeSetToZoneRequests(changeSet, doneHandler)
}

func convertChangeSetToZoneRequests(changeSet changeSet, doneHandler provider.DoneHandler) zonedRequests {
	if len(changeSet.additions)+len(changeSet.updates)+len(changeSet.deletions) == 0 {
		return nil // No changes to apply
	}

	zoneRequests := make(zonedRequests)
	addChangeRequest := func(key records.FullRecordSetKey, old, new *dns.RecordSet) {
		m := zoneRequests[key.ZoneID]
		if m == nil {
			m = make(map[dns.DNSSetName]*provider.ChangeRequests)
			zoneRequests[key.ZoneID] = m
		}
		reqs := m[key.Name]
		if reqs == nil {
			reqs = provider.NewChangeRequests(key.Name, doneHandler)
			m[key.Name] = reqs
		}
		reqs.Updates[key.RecordType] = &provider.ChangeRequestUpdate{Old: old, New: new}
	}

	for key, recordSet := range changeSet.additions {
		addChangeRequest(key, nil, recordSet)
	}
	for key, pair := range changeSet.updates {
		addChangeRequest(key, pair.old, pair.new)
	}
	for key, recordSet := range changeSet.deletions {
		addChangeRequest(key, recordSet, nil)
	}
	return zoneRequests
}

func toDNSSetName(dnsName string, routingPolicy *v1alpha1.RoutingPolicy) (dns.DNSSetName, *dns.RoutingPolicy, error) {
	var setIdentifier string
	if routingPolicy != nil {
		setIdentifier = routingPolicy.SetIdentifier
	}
	convertedRoutingPolicy, err := convertRoutingPolicy(routingPolicy)
	if err != nil {
		return dns.DNSSetName{}, nil, fmt.Errorf("failed to convert routing policy: %w", err)
	}

	return dns.DNSSetName{
		DNSName:       dnsName,
		SetIdentifier: setIdentifier,
	}, convertedRoutingPolicy, nil
}

func convertRoutingPolicy(rp *v1alpha1.RoutingPolicy) (*dns.RoutingPolicy, error) {
	if rp == nil {
		return nil, nil
	}

	var convertedType dns.RoutingPolicyType
	for _, typ := range dns.AllRoutingPolicyTypes {
		if string(typ) == rp.Type {
			convertedType = typ
			break
		}
	}
	if convertedType == "" {
		return nil, fmt.Errorf("unknown routing policy type %q", rp.Type)
	}
	return &dns.RoutingPolicy{
		Type:       convertedType,
		Parameters: rp.Parameters,
	}, nil
}

func ignoreByAnnotation(entry *v1alpha1.DNSEntry) (string, bool) {
	if entry.Annotations[dns.AnnotationHardIgnore] == "true" {
		return dns.AnnotationHardIgnore + "=true", true
	}
	if entry.Annotations[dns.AnnotationIgnore] == dns.AnnotationIgnoreValueFull {
		return dns.AnnotationIgnore + "=" + dns.AnnotationIgnoreValueFull, true
	}
	if entry.DeletionTimestamp == nil {
		if value := entry.Annotations[dns.AnnotationIgnore]; value == dns.AnnotationIgnoreValueReconcile || value == dns.AnnotationIgnoreValueTrue {
			return dns.AnnotationIgnore + "=" + value, true
		}
	}

	return "", false
}
