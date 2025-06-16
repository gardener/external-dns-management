// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"context"
	"fmt"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/lookup"
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

type fullDNSSetName struct {
	zoneID dns.ZoneID
	name   dns.DNSSetName
}

type fullRecordSetKey struct {
	fullDNSSetName
	recordType dns.RecordType
}

type fullRecordKeySet = sets.Set[fullRecordSetKey]

type entryContext struct {
	client client.Client
	clock  clock.Clock
	ctx    context.Context
	log    logr.Logger
	entry  *v1alpha1.DNSEntry
}

func (ec *entryContext) statusUpdater() *entryStatusUpdater {
	return &entryStatusUpdater{entryContext: *ec}
}

type entryReconciliation struct {
	entryContext
	namespace       string
	class           string
	state           *state.State
	lookupProcessor lookup.LookupProcessor
}

type reconcileResult struct {
	result reconcile.Result
	err    error
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

func (r *entryReconciliation) reconcile() reconcileResult {
	if res := r.ignoredByAnnotation(); res != nil {
		return *res
	}

	newProviderData, res := r.providerSelector().calcNewProvider()
	if res != nil {
		return *res
	}

	recordsToCheck := fullRecordKeySet{}
	if res := r.calcOldTargets(recordsToCheck); res != nil {
		return *res
	}

	if newProviderData == nil {
		return r.statusUpdater().updateStatusWithoutProvider()
	}

	if res := r.checkProviderNotReady(newProviderData); res != nil {
		return *res
	}

	targetsData, res := r.calcNewTargets(newProviderData, recordsToCheck)
	if res != nil {
		return *res
	}

	actualRecords, res := r.queryRecords(recordsToCheck)
	if res != nil {
		return *res
	}

	var doneHandler provider.DoneHandler // TODO(MartinWeindel): Implement a done handler if needed
	zonedRequests := calculateZonedChangeRequests(targetsData, doneHandler, actualRecords)

	if res := r.applyChangeRequests(newProviderData, zonedRequests); res != nil {
		return *res
	}

	return r.statusUpdater().updateStatusWithProvider(targetsData)
}

func (r *entryReconciliation) providerSelector() *providerSelector {
	return &providerSelector{
		entryContext: r.entryContext,
		namespace:    r.namespace,
		class:        r.class,
		state:        r.state,
	}
}

func (r *entryReconciliation) ignoredByAnnotation() *reconcileResult {
	if annotation, b := ignoreByAnnotation(r.entry); b {
		r.log.V(1).Info("Ignoring entry due to annotation", "annotation", annotation)
		if r.entry.DeletionTimestamp != nil {
			if res := r.statusUpdater().removeFinalizer(); res != nil {
				return res
			}
		}
		return &reconcileResult{}
	}
	return nil
}

func (r *entryReconciliation) calcOldTargets(recordsToCheck fullRecordKeySet) *reconcileResult {
	status := r.entry.Status
	oldTargets, err := StatusToTargets(&status, r.entry.Annotations[dns.AnnotationIPStack])
	if err != nil {
		// should not happen, but if it does, we want to log it
		return &reconcileResult{err: err}
	}
	if status.ProviderType != nil && status.Zone != nil && status.DNSName != nil {
		name, _, err := toDNSSetName(*status.DNSName, status.RoutingPolicy)
		if err != nil {
			return &reconcileResult{err: err}
		}
		oldZoneID := &dns.ZoneID{ID: *status.Zone, ProviderType: *status.ProviderType}
		// targets from status are already mapped to the provider, so we can use them directly
		insertRecordKeys(recordsToCheck, fullDNSSetName{zoneID: *oldZoneID, name: name}, oldTargets)
	}
	return nil
}

func (r *entryReconciliation) checkProviderNotReady(data *newProviderData) *reconcileResult {
	if data.provider.Status.State != v1alpha1.StateReady {
		var err error
		if data.provider.Status.State != "" {
			err = fmt.Errorf("provider %s has status %s: %s", data.providerKey, data.provider.Status.State, ptr.Deref(data.provider.Status.Message, "unknown error"))
		} else {
			err = fmt.Errorf("provider %s is not ready yet", data.providerKey)
		}
		res := r.statusUpdater().failWithStatusStale(err)
		return &res
	}
	return nil
}

func (r *entryReconciliation) calcNewTargets(providerData *newProviderData, recordsToCheck fullRecordKeySet) (*newTargetsData, *reconcileResult) {
	producer := &TargetsProducer{
		ctx:                        r.ctx,
		defaultTTL:                 providerData.defaultTTL,
		defaultCNAMELookupInterval: 600,
		processor:                  r.lookupProcessor,
	}
	result, err := producer.FromSpec(client.ObjectKeyFromObject(r.entry), &r.entry.Spec, r.entry.Annotations[dns.AnnotationIPStack])
	if err != nil {
		return nil, r.statusUpdater().updateStatusInvalid(err)
	}
	name, rp, err := toDNSSetName(r.entry.Spec.DNSName, r.entry.Spec.RoutingPolicy)
	if err != nil {
		return nil, &reconcileResult{err: err}
	}

	account := providerData.providerState.GetAccount()
	if account == nil {
		res := r.statusUpdater().failWithStatusError(fmt.Errorf("provider %s is not ready yet", providerData.providerKey))
		return nil, &res
	}
	newTargets := account.MapTargets(r.entry.Spec.DNSName, result.Targets)
	newDNSSet := dns.NewDNSSet(name)
	newZoneID := providerData.zoneID
	insertRecordKeys(recordsToCheck, fullDNSSetName{zoneID: newZoneID, name: name}, newTargets)
	if r.entry.DeletionTimestamp == nil {
		insertRecordSets(*newDNSSet, rp, newTargets)
	} else {
		newTargets = nil
	}
	return &newTargetsData{
		providerKey:   providerData.providerKey,
		providerType:  providerData.provider.Spec.Type,
		zoneID:        newZoneID,
		dnsSet:        newDNSSet,
		targets:       newTargets,
		routingPolicy: rp,
		warnings:      result.Warnings,
	}, nil
}

func (r *entryReconciliation) applyChangeRequests(providerData *newProviderData, zonedRequests zonedRequests) *reconcileResult {
	newZoneID := providerData.zoneID
	for zoneID, perName := range zonedRequests {
		if zoneID == newZoneID {
			continue
		}
		providerAccount := providerData.providerState.GetAccount()
		if res := r.cleanupCrossZoneRecords(zoneID, perName, providerAccount); res != nil {
			return res
		}
	}

	changeRequestsPerName := zonedRequests[newZoneID]
	if len(changeRequestsPerName) > 0 {
		zones, err := providerData.providerState.GetAccount().GetZones(r.ctx)
		if err != nil {
			r.log.Error(err, "failed to get zones from DNS account", "provider", providerData.providerKey)
			return &reconcileResult{err: err}
		}
		var zone provider.DNSHostedZone
		for _, z := range zones {
			if z.ZoneID() == newZoneID {
				zone = z
				break
			}
		}
		if zone == nil {
			err := fmt.Errorf("zone %s not found in provider %s", newZoneID.ID, providerData.providerKey)
			r.log.Error(err, "failed to find zone for DNS entry")
			return &reconcileResult{err: err}
		}
		for _, changeRequests := range changeRequestsPerName {
			if err := providerData.providerState.GetAccount().ExecuteRequests(r.ctx, zone, *changeRequests); err != nil {
				r.log.Error(err, "failed to execute DNS change requests", "provider", providerData.providerKey)
				return &reconcileResult{err: err}
			}
		}
	}
	return nil
}

func (r *entryReconciliation) queryRecords(keys fullRecordKeySet) (map[fullRecordSetKey]*dns.RecordSet, *reconcileResult) {
	zonesToCheck := sets.Set[dns.ZoneID]{}
	for key := range keys {
		zonesToCheck.Insert(key.zoneID)
	}

	results := make(map[fullRecordSetKey]*dns.RecordSet)
	for zoneID := range zonesToCheck {
		queryHandler, err := r.state.GetDNSQueryHandler(zoneID)
		if err != nil {
			r.log.Error(err, "failed to get DNS query handler for zone", "zoneID", zoneID.ID)
			return nil, &reconcileResult{err: fmt.Errorf("failed to get query handler for zone %s: %w", zoneID.ID, err)}
		}
		for key := range keys {
			if key.zoneID != zoneID {
				continue
			}
			targets, policy, err := queryHandler.Query(r.ctx, key.name.DNSName, key.name.SetIdentifier, key.recordType)
			if err != nil {
				r.log.Error(err, "failed to query DNS records", "name", key.name, "type", key.recordType, "zoneID", zoneID.ID)
				return nil, &reconcileResult{err: fmt.Errorf("failed to query DNS records for %s, type %s in zone %s: %w", key.name, key.recordType, zoneID.ID, err)}
			}
			if len(targets) > 0 {
				dnsSet := dns.NewDNSSet(key.name)
				insertRecordSets(*dnsSet, policy, targets)
				results[key] = dnsSet.Sets[key.recordType]
			}
		}
	}
	return results, nil
}

func (r *entryReconciliation) cleanupCrossZoneRecords(zoneID dns.ZoneID, perName map[dns.DNSSetName]*provider.ChangeRequests, account *provider.DNSAccount) *reconcileResult {
	if len(perName) == 0 {
		return nil // Nothing to clean up
	}

	var zone *provider.DNSHostedZone
	zones, err := account.GetZones(r.ctx)
	if err != nil {
		r.log.Error(err, "failed to get zones from DNS account")
		return &reconcileResult{err: err}
	}
	for _, z := range zones {
		if z.ZoneID() == zoneID {
			zone = &z
			break
		}
	}
	if zone == nil {
		account, zone, err = r.state.FindAccountForZone(r.ctx, zoneID) // Ensure the account is loaded for the zone
		if err != nil {
			r.log.Error(err, "failed to find account for zone", "zoneID", zoneID)
			res := r.statusUpdater().failWithStatusError(fmt.Errorf("failed to find account for old zone %q to clean up old records", zoneID))
			return &res
		}
	}

	for name, changeRequests := range perName {
		if len(changeRequests.Updates) == 0 {
			continue // Nothing to delete for this name
		}
		r.log.Info("Deleting cross-zone records", "zoneID", zoneID, "name", name)
		if err := account.ExecuteRequests(r.ctx, *zone, *changeRequests); err != nil {
			r.log.Error(err, "failed to delete cross-zone records", "zoneID", zoneID, "name", name)
			return &reconcileResult{err: err}
		}
	}
	return nil
}

func insertRecordKeys(keys fullRecordKeySet, fullDNSSetName fullDNSSetName, targets dns.Targets) {
	for _, target := range targets {
		keys.Insert(fullRecordSetKey{
			fullDNSSetName: fullDNSSetName,
			recordType:     target.GetRecordType(),
		})
	}
}

type updatePair struct {
	old *dns.RecordSet
	new *dns.RecordSet
}
type changeSet struct {
	additions map[fullRecordSetKey]*dns.RecordSet
	updates   map[fullRecordSetKey]*updatePair
	deletions map[fullRecordSetKey]*dns.RecordSet
}

func calculateZonedChangeRequests(targetsData *newTargetsData, doneHandler provider.DoneHandler, actual map[fullRecordSetKey]*dns.RecordSet) zonedRequests {
	changeSet := changeSet{
		additions: make(map[fullRecordSetKey]*dns.RecordSet),
		updates:   make(map[fullRecordSetKey]*updatePair),
		deletions: make(map[fullRecordSetKey]*dns.RecordSet),
	}

	newDNSSet := targetsData.dnsSet
	newFullDNSSetName := fullDNSSetName{name: newDNSSet.Name, zoneID: targetsData.zoneID}
	for key, recordSet := range actual {
		if key.fullDNSSetName != newFullDNSSetName || newDNSSet.Sets[key.recordType] == nil {
			changeSet.deletions[key] = recordSet
		} else if !newDNSSet.Sets[key.recordType].Match(recordSet) {
			changeSet.updates[key] = &updatePair{
				old: recordSet,
				new: newDNSSet.Sets[key.recordType],
			}
		}
	}
	for rtype, recordSet := range newDNSSet.Sets {
		rsKey := fullRecordSetKey{
			fullDNSSetName: newFullDNSSetName,
			recordType:     rtype,
		}
		if _, exists := actual[rsKey]; !exists {
			changeSet.additions[rsKey] = recordSet
		}
	}

	return convertChangeSetToZoneRequests(changeSet, doneHandler)
}

func convertChangeSetToZoneRequests(changeSet changeSet, doneHandler provider.DoneHandler) zonedRequests {
	if len(changeSet.additions)+len(changeSet.updates)+len(changeSet.deletions) == 0 {
		return nil // No changes to apply
	}

	zoneRequests := make(zonedRequests)
	addChangeRequest := func(key fullRecordSetKey, old, new *dns.RecordSet) {
		m := zoneRequests[key.zoneID]
		if m == nil {
			m = make(map[dns.DNSSetName]*provider.ChangeRequests)
			zoneRequests[key.zoneID] = m
		}
		reqs := m[key.name]
		if reqs == nil {
			reqs = provider.NewChangeRequests(key.name, doneHandler)
			m[key.name] = reqs
		}
		reqs.Updates[key.recordType] = &provider.ChangeRequestUpdate{Old: old, New: new}
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

func insertRecordSets(dnsSet dns.DNSSet, policy *dns.RoutingPolicy, targets dns.Targets) {
	recordSets := map[dns.RecordType]*dns.RecordSet{}
	for _, target := range targets {
		if _, exists := recordSets[target.GetRecordType()]; !exists {
			recordSets[target.GetRecordType()] = &dns.RecordSet{
				Type:          target.GetRecordType(),
				TTL:           target.GetTTL(),
				RoutingPolicy: policy,
			}
		}
		recordSets[target.GetRecordType()].Records = append(recordSets[target.GetRecordType()].Records, target.AsRecord())
	}
	for rtype, recordSet := range recordSets {
		dnsSet.Sets[rtype] = recordSet
	}
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
