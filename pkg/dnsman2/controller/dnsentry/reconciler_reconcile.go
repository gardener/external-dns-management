// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/selection"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
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

func (r *Reconciler) reconcile(ctx context.Context, log logr.Logger, entry *v1alpha1.DNSEntry) (reconcile.Result, error) {
	if annotation, b := ignoreByAnnotation(entry); b {
		log.V(1).Info("Ignoring entry due to annotation", "annotation", annotation)
		if entry.DeletionTimestamp != nil {
			if err := removeFinalizer(ctx, r.Client, entry); err != nil {
				log.Error(err, "failed to remove finalizer to DNSEntry", "entry", entry.Name)
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}
	ctx = logr.NewContext(ctx, log)
	newProvider, err := r.findBestMatchingProvider(ctx, entry.Spec.DNSName, entry.Status.Provider)
	if err != nil {
		log.Error(err, "failed to find a matching DNS provider for the entry", "entry", entry.Name)
		return reconcile.Result{}, err
	}
	var (
		newZone       *string
		providerState *state.ProviderState
		defaultTTL    int64
	)
	if newProvider != nil {
		var result *reconcile.Result
		newZone, result, err = r.getZoneForProvider(newProvider, entry.Spec.DNSName)
		if result != nil {
			log.Error(err, "failed to get zone for provider", "provider", client.ObjectKeyFromObject(newProvider))
			return *result, err
		}
		if err != nil {
			log.Error(err, "failed to get zone for provider", "provider", client.ObjectKeyFromObject(newProvider))
			return reconcile.Result{}, err
		}
		if err := addFinalizer(ctx, r.Client, entry); err != nil {
			log.Error(err, "failed to add finalizer to DNSEntry")
			return reconcile.Result{}, err
		}
		providerState = r.state.GetProviderState(client.ObjectKeyFromObject(newProvider))
		if providerState == nil {
			log.Error(err, "failed to get provider state", "provider", client.ObjectKeyFromObject(newProvider))
			return reconcile.Result{}, err
		}
		defaultTTL = providerState.GetDefaultTTL()
	}

	recordsToCheck := fullRecordKeySet{}
	oldTargets, err := StatusToTargets(&entry.Status, entry.Annotations[dns.AnnotationIPStack])
	if err != nil {
		// should not happen, but if it does, we want to log it
		return reconcile.Result{}, err
	}
	if entry.Status.ProviderType != nil && entry.Status.Zone != nil && entry.Status.DNSName != nil {
		name, _, err := toDNSSetName(*entry.Status.DNSName, entry.Status.RoutingPolicy)
		if err != nil {
			return reconcile.Result{}, err
		}
		oldZoneID := &dns.ZoneID{ID: *entry.Status.Zone, ProviderType: *entry.Status.ProviderType}
		// targets from status are already mapped to the provider, so we can use them directly
		insertRecordKeys(recordsToCheck, fullDNSSetName{zoneID: *oldZoneID, name: name}, oldTargets)
	}

	if newProvider == nil || newZone == nil || !providerState.IsValid() {
		if err := r.updateStatus(ctx, entry, func(status *v1alpha1.DNSEntryStatus) error {
			status.Provider = nil
			status.ObservedGeneration = entry.Generation
			if len(status.Targets) > 0 && status.Zone != nil {
				status.State = v1alpha1.StateStale
			} else {
				status.State = v1alpha1.StateError
				status.ProviderType = nil
			}
			status.Message = ptr.To("no matching DNS provider found")
			return nil
		}); err != nil {
			log.Error(err, "failed to update DNSEntry status", "entry", entry.Name)
			return reconcile.Result{}, err
		}
		if len(entry.Status.Targets) == 0 {
			if err := removeFinalizer(ctx, r.Client, entry); err != nil {
				log.Error(err, "failed to remove finalizer to DNSEntry", "entry", entry.Name)
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil // No provider or zone found, nothing to do
	}

	if newProvider.Status.State != v1alpha1.StateReady {
		var err error
		if newProvider.Status.State != "" {
			err = fmt.Errorf("provider %s has status %s: %s", client.ObjectKeyFromObject(newProvider), newProvider.Status.State, ptr.Deref(newProvider.Status.Message, "unknown error"))
		} else {
			err = fmt.Errorf("provider %s is not ready yet", client.ObjectKeyFromObject(newProvider))
		}
		return r.failWithStatusStale(ctx, log, entry, err)
	}

	rawNewTargets, warnings, err := SpecToTargets(client.ObjectKeyFromObject(entry), &entry.Spec, entry.Annotations[dns.AnnotationIPStack], defaultTTL)
	if err != nil {
		err2 := r.updateStatusInvalid(ctx, entry, err)
		return reconcile.Result{}, err2
	}
	name, rp, err := toDNSSetName(entry.Spec.DNSName, entry.Spec.RoutingPolicy)
	if err != nil {
		return reconcile.Result{}, err
	}

	account := providerState.GetAccount()
	if account == nil {
		return r.failWithStatusError(ctx, log, entry, fmt.Errorf("provider %s is not ready yet", client.ObjectKeyFromObject(newProvider)))
	}
	newTargets := account.MapTargets(entry.Spec.DNSName, rawNewTargets)
	newDNSSet := dns.NewDNSSet(name)
	newZoneID := dns.ZoneID{ProviderType: newProvider.Spec.Type, ID: *newZone}
	insertRecordKeys(recordsToCheck, fullDNSSetName{zoneID: newZoneID, name: name}, newTargets)
	if entry.DeletionTimestamp == nil {
		insertRecordSets(*newDNSSet, rp, newTargets)
	} else {
		newTargets = nil
	}

	actualRecords, err := r.queryRecords(ctx, recordsToCheck)
	if err != nil {
		log.Error(err, "failed to query DNS records")
		return reconcile.Result{}, err
	}

	var doneHandler provider.DoneHandler // TODO(MartinWeindel): Implement a done handler if needed
	zonedRequests := calculateZonedChangeRequests(*newDNSSet, doneHandler, newZoneID, actualRecords)
	for zoneID, perName := range zonedRequests {
		if zoneID == newZoneID {
			continue
		}
		providerAccount := providerState.GetAccount()
		result, err := r.cleanupCrossZoneRecords(ctx, log, entry, zoneID, perName, providerAccount)
		if err != nil {
			return result, err
		}
	}

	changeRequestsPerName := zonedRequests[newZoneID]
	if len(changeRequestsPerName) > 0 {
		zones, err := providerState.GetAccount().GetZones(ctx)
		if err != nil {
			log.Error(err, "failed to get zones from DNS account", "provider", client.ObjectKeyFromObject(newProvider))
			return reconcile.Result{}, err
		}
		var zone provider.DNSHostedZone
		for _, z := range zones {
			if z.ZoneID() == newZoneID {
				zone = z
				break
			}
		}
		if zone == nil {
			err := fmt.Errorf("zone %s not found in provider %s", newZoneID.ID, client.ObjectKeyFromObject(newProvider))
			log.Error(err, "failed to find zone for DNS entry")
			return reconcile.Result{}, err
		}
		for _, changeRequests := range changeRequestsPerName {
			if err := providerState.GetAccount().ExecuteRequests(ctx, zone, *changeRequests); err != nil {
				log.Error(err, "failed to execute DNS change requests", "provider", client.ObjectKeyFromObject(newProvider))
				return reconcile.Result{}, err
			}
		}
	}

	if err := r.updateStatus(ctx, entry, func(status *v1alpha1.DNSEntryStatus) error {
		status.Provider = &newProvider.Name
		status.ProviderType = ptr.To(newProvider.Spec.Type)
		status.Zone = newZone
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
		status.Targets = TargetsToStrings(newTargets)
		if len(newTargets) > 0 {
			status.TTL = ptr.To(newTargets[0].GetTTL())
		} else {
			status.TTL = nil
		}

		status.ObservedGeneration = entry.Generation
		status.State = v1alpha1.StateReady
		if len(warnings) > 0 {
			status.Message = ptr.To(fmt.Sprintf("reconciled with warnings: %s", strings.Join(warnings, ", ")))
		} else {
			status.Message = ptr.To("dns entry active")
		}
		return nil
	}); err != nil {
		log.Error(err, "failed to update DNSEntry status", "entry", entry.Name)
		return reconcile.Result{}, err
	}

	if len(entry.Status.Targets) == 0 {
		if err := removeFinalizer(ctx, r.Client, entry); err != nil {
			log.Error(err, "failed to remove finalizer to DNSEntry", "entry", entry.Name)
			return reconcile.Result{}, err
		}
	}

	_ = warnings // TODO: handle warnings if necessary
	// TODO: implement logic
	return reconcile.Result{}, nil
}

func (r *Reconciler) findBestMatchingProvider(ctx context.Context, dnsName string, currentProviderName *string) (*v1alpha1.DNSProvider, error) {
	providerList := &v1alpha1.DNSProviderList{}
	if err := r.Client.List(ctx, providerList, client.InNamespace(r.Namespace)); err != nil {
		return nil, err
	}
	return findBestMatchingProvider(dns.FilterProvidersByClass(providerList.Items, r.Class), dnsName, currentProviderName)
}

func (r *Reconciler) getZoneForProvider(provider *v1alpha1.DNSProvider, dnsName string) (*string, *reconcile.Result, error) {
	pstate := r.state.GetProviderState(client.ObjectKeyFromObject(provider))
	if pstate == nil {
		return nil, &reconcile.Result{Requeue: true}, nil // Provider state not yet available, requeue to wait for its reconciliation
	}
	var (
		bestZone  selection.LightDNSHostedZone
		bestMatch int
	)
	for _, zone := range pstate.GetSelection().Zones {
		if m := matchDomains(dnsName, []string{zone.Domain()}); m > bestMatch {
			bestMatch = m
			bestZone = zone
		}
	}
	if bestMatch == 0 {
		return nil, nil, fmt.Errorf("no matching zone found for DNS name %q in provider %q", dnsName, provider.Name)
	}
	return ptr.To(bestZone.ZoneID().ID), nil, nil
}

func (r *Reconciler) queryRecords(ctx context.Context, keys fullRecordKeySet) (map[fullRecordSetKey]*dns.RecordSet, error) {
	zonesToCheck := sets.Set[dns.ZoneID]{}
	for key := range keys {
		zonesToCheck.Insert(key.zoneID)
	}

	results := make(map[fullRecordSetKey]*dns.RecordSet)
	for zoneID := range zonesToCheck {
		queryHandler, err := r.state.GetDNSQueryHandler(zoneID)
		if err != nil {
			return nil, fmt.Errorf("failed to get query handler for zone %s: %w", zoneID.ID, err)
		}
		for key := range keys {
			if key.zoneID != zoneID {
				continue
			}
			targets, policy, err := queryHandler.Query(ctx, key.name.DNSName, key.name.SetIdentifier, key.recordType)
			if err != nil {
				return nil, fmt.Errorf("failed to query DNS records for %s, type %s in zone %s: %w", key.name, key.recordType, zoneID.ID, err)
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

func (r *Reconciler) cleanupCrossZoneRecords(ctx context.Context, log logr.Logger, entry *v1alpha1.DNSEntry, zoneID dns.ZoneID, perName map[dns.DNSSetName]*provider.ChangeRequests, account *provider.DNSAccount) (reconcile.Result, error) {
	if len(perName) == 0 {
		return reconcile.Result{}, nil // Nothing to clean up
	}

	var zone *provider.DNSHostedZone
	zones, err := account.GetZones(ctx)
	if err != nil {
		log.Error(err, "failed to get zones from DNS account")
		return reconcile.Result{}, err
	}
	for _, z := range zones {
		if z.ZoneID() == zoneID {
			zone = &z
			break
		}
	}
	if zone == nil {
		account, zone, err = r.state.FindAccountForZone(ctx, zoneID) // Ensure the account is loaded for the zone
		if err != nil {
			log.Error(err, "failed to find account for zone", "zoneID", zoneID)
			return r.failWithStatusError(ctx, log, entry, fmt.Errorf("failed to find account for old zone %q to clean up old records", zoneID))
		}
	}

	for name, changeRequests := range perName {
		if len(changeRequests.Updates) == 0 {
			continue // Nothing to delete for this name
		}
		log.Info("Deleting cross-zone records", "zoneID", zoneID, "name", name)
		if err := account.ExecuteRequests(ctx, *zone, *changeRequests); err != nil {
			log.Error(err, "failed to delete cross-zone records", "zoneID", zoneID, "name", name)
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
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

func calculateZonedChangeRequests(newDNSSet dns.DNSSet, doneHandler provider.DoneHandler, newZone dns.ZoneID, actual map[fullRecordSetKey]*dns.RecordSet) map[dns.ZoneID]map[dns.DNSSetName]*provider.ChangeRequests {
	changeSet := changeSet{
		additions: make(map[fullRecordSetKey]*dns.RecordSet),
		updates:   make(map[fullRecordSetKey]*updatePair),
		deletions: make(map[fullRecordSetKey]*dns.RecordSet),
	}

	newFullDNSSetName := fullDNSSetName{name: newDNSSet.Name, zoneID: newZone}
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

func convertChangeSetToZoneRequests(changeSet changeSet, doneHandler provider.DoneHandler) map[dns.ZoneID]map[dns.DNSSetName]*provider.ChangeRequests {
	if len(changeSet.additions)+len(changeSet.updates)+len(changeSet.deletions) == 0 {
		return nil // No changes to apply
	}

	zoneRequests := make(map[dns.ZoneID]map[dns.DNSSetName]*provider.ChangeRequests)
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
		if value := entry.Annotations[dns.AnnotationIgnore];
			value == dns.AnnotationIgnoreValueReconcile || value == dns.AnnotationIgnoreValueTrue {
			return dns.AnnotationIgnore + "=" + value, true
		}
	}

	return "", false
}
