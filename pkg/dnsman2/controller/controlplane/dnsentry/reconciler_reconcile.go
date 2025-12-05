// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"fmt"
	"strings"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/lookup"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/providerselector"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/records"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/targets"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type entryReconciliation struct {
	common.EntryContext
	namespace                  string
	class                      string
	migrationMode              bool
	state                      *state.State
	lookupProcessor            lookup.LookupProcessor
	defaultCNAMELookupInterval int64
	lastUpdate                 *ttlcache.Cache[client.ObjectKey, struct{}]
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
	r.Log.Info("reconcile")
	orgResult := r.doReconcile()
	res := orgResult

	// update status if state/message changed
	if orgResult.State != nil {
		res = r.StatusUpdater().UpdateStatus(func(status *v1alpha1.DNSEntryStatus) error {
			status.ObservedGeneration = r.Entry.Generation
			status.State = *orgResult.State
			status.Message = orgResult.Message
			return nil
		})
		if res.Result.IsZero() {
			res.Result = orgResult.Result
		}
	}
	return res
}

func (r *entryReconciliation) doReconcile() common.ReconcileResult {
	if res := r.ignoredByAnnotation(); res != nil {
		return *res
	}

	unlockFunc, res := r.lockDNSNames()
	if res != nil {
		return *res
	}
	defer unlockFunc()

	if err := validateDNSEntry(r.Entry); err != nil {
		return *common.InvalidReconcileResult(fmt.Sprintf("validation failed: %s", err))
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

	actualRecords, res := r.dnsRecordManager().QueryRecords(r.Ctx, recordsToCheck)
	if res != nil {
		return *res
	}

	zonedRequests := calculateZonedChangeRequests(targetsData, actualRecords)
	if len(zonedRequests) > 0 {
		r.clearDNSCaches(zonedRequests)
		r.lastUpdate.Set(client.ObjectKeyFromObject(r.Entry), struct{}{}, ttlcache.DefaultTTL)
	}
	if res := r.dnsRecordManager().ApplyChangeRequests(newProviderData, zonedRequests); res != nil {
		return *res
	}

	return r.updateStatusWithProvider(targetsData)
}

func (r *entryReconciliation) lockDNSNames() (func(), *common.ReconcileResult) {
	names := getDNSNames(r.Entry.Spec.DNSName, r.Entry.Status.DNSName)
	locking := r.state.GetDNSNameLocking()
	if !locking.Lock(names...) {
		// already locked by another entry, requeue
		return nil, &common.ReconcileResult{
			Result: reconcile.Result{RequeueAfter: 3*time.Second + time.Duration(rand.Intn(500))*time.Millisecond},
		}
	}
	return func() {
		locking.Unlock(names...)
	}, nil
}

func validateDNSEntry(entry *v1alpha1.DNSEntry) error {
	if err := dns.ValidateDomainName(entry.Spec.DNSName); err != nil {
		return fmt.Errorf("invalid DNSName: %w", err)
	}

	if len(entry.Spec.Targets) > 0 && len(entry.Spec.Text) > 0 {
		return fmt.Errorf("cannot specify both targets and text fields")
	}

	recordData := map[string]struct{}{}
	for i, target := range entry.Spec.Targets {
		if strings.TrimSpace(target) == "" {
			return fmt.Errorf("target %d is empty", i+1)
		}

		if _, exists := recordData[target]; exists {
			return fmt.Errorf("target %d is a duplicate: %s", i+1, target)
		}

		recordData[target] = struct{}{}
	}

	for i, text := range entry.Spec.Text {
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("text %d is empty", i+1)
		}

		if _, exists := recordData[text]; exists {
			return fmt.Errorf("text %d is a duplicate: %s", i+1, text)
		}

		recordData[text] = struct{}{}
	}

	return nil
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
		status.Provider = ptr.To(targetsData.providerKey.String())
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
			status.Message = ptr.To("dns entry active")
		}
		return nil
	})
}

func (r *entryReconciliation) ignoredByAnnotation() *common.ReconcileResult {
	annotation, ignoreFully := dns.IgnoreFullByAnnotation(r.Entry)
	ignore := ignoreFully
	if !ignoreFully {
		annotation, ignore = dns.IgnoreReconcileByAnnotation(r.Entry)
		ignore = ignore && r.Entry.DeletionTimestamp == nil
	}
	if ignore {
		r.Log.V(1).Info("Ignoring Entry due to annotation", "annotation", annotation)
		if ignoreFully {
			if res := r.StatusUpdater().RemoveFinalizer(); res != nil {
				return res
			}
		}
		return &common.ReconcileResult{
			State: ptr.To(v1alpha1.StateIgnored),
			Message: ptr.To(fmt.Sprintf("entry is ignored due to annotation: %s",
				annotation)),
		}
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
		if r.migrationMode && ptr.Deref(status.ProviderType, "") == "remote" {
			// in migration mode, we ignore the unsupported "remote" provider type
			// they will be handled by the real provider instead
			return nil
		}
		name, _, err := toDNSSetName(*status.DNSName, status.RoutingPolicy)
		if err != nil {
			return common.ErrorReconcileResult(err.Error(), false)
		}
		oldZoneID := &dns.ZoneID{ID: *status.Zone, ProviderType: *status.ProviderType}
		oldMappedTargets, err := r.mapTargets(oldTargets, *status.ProviderType)
		if err != nil {
			return common.ErrorReconcileResult(fmt.Sprintf("failed to map old targets: %s", err), false)
		}
		records.InsertRecordKeys(recordsToCheck, records.FullDNSSetName{ZoneID: *oldZoneID, Name: name}, oldMappedTargets)
	}
	return nil
}

func (r *entryReconciliation) checkProviderNotReady(data *providerselector.NewProviderData) *common.ReconcileResult {
	if data.Provider.Status.State != v1alpha1.StateReady {
		var msg string
		if data.Provider.Status.State != "" {
			msg = fmt.Sprintf("provider %s has status %s: %s", data.ProviderKey, data.Provider.Status.State, ptr.Deref(data.Provider.Status.Message, "unknown error"))
		} else {
			msg = fmt.Sprintf("provider %s is not ready yet", data.ProviderKey)
		}
		return common.StaleReconcileResult(msg, false)
	}
	return nil
}

func (r *entryReconciliation) calcNewTargets(providerData *providerselector.NewProviderData, recordsToCheck records.FullRecordKeySet) (*newTargetsData, *common.ReconcileResult) {
	producer := targets.NewTargetsProducer(r.Ctx, providerData.DefaultTTL, r.defaultCNAMELookupInterval, r.lookupProcessor)

	result, err := producer.FromSpec(client.ObjectKeyFromObject(r.Entry), &r.Entry.Spec, r.Entry.Annotations[dns.AnnotationIPStack])
	if err != nil {
		return nil, common.InvalidReconcileResult(err.Error())
	}
	name, rp, err := toDNSSetName(r.Entry.Spec.DNSName, r.Entry.Spec.RoutingPolicy)
	if err != nil {
		return nil, common.ErrorReconcileResult(err.Error(), false)
	}

	newTargets, err := r.mapTargets(result.Targets, providerData.Provider.Spec.Type)
	if err != nil {
		return nil, common.ErrorReconcileResult(err.Error(), false)
	}

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

func (r *entryReconciliation) mapTargets(targets dns.Targets, providerType string) (dns.Targets, error) {
	mapper, err := r.state.GetDNSHandlerFactory().GetTargetsMapper(providerType)
	if err != nil {
		return nil, err
	}
	if mapper == nil {
		return targets, nil
	}
	return makeTargetsUnique(mapper.MapTargets(targets)), nil
}

// clearDNSCaches clears the DNS caches for the zones that are being updated
func (r *entryReconciliation) clearDNSCaches(zonedRequests zonedRequests) {
	for zoneID, requests := range zonedRequests {
		var keys []utils.RecordSetKey
		for name, changes := range requests {
			for recordType := range changes.Updates {
				keys = append(keys, utils.RecordSetKey{Name: name, RecordType: recordType})
			}
		}
		if err := r.state.ClearDNSCaches(r.Ctx, zoneID, keys...); err != nil {
			r.Log.Error(err, "failed to clear DNS caches for zone", "zoneID", zoneID.ID)
		}
	}
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

func calculateZonedChangeRequests(targetsData *newTargetsData, actual map[records.FullRecordSetKey]*dns.RecordSet) zonedRequests {
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

	return convertChangeSetToZoneRequests(changeSet)
}

func convertChangeSetToZoneRequests(changeSet changeSet) zonedRequests {
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
			reqs = provider.NewChangeRequests(key.Name)
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

func makeTargetsUnique(targets dns.Targets) dns.Targets {
	uniqueTargets := make(dns.Targets, 0, len(targets))
	for _, target := range targets {
		if uniqueTargets.Has(target) {
			continue
		}
		uniqueTargets = append(uniqueTargets, target)
	}
	return uniqueTargets
}

func getDNSNames(dnsName string, statusDNSName *string) []string {
	if statusDNSName != nil && *statusDNSName != "" && dnsName != *statusDNSName {
		return []string{dnsName, *statusDNSName}
	}
	return []string{dnsName}
}
