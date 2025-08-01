// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
	"github.com/gardener/external-dns-management/pkg/dns/provider/zonetxn"
	"github.com/gardener/external-dns-management/pkg/server/metrics"
)

////////////////////////////////////////////////////////////////////////////////
// state handling for zone reconcilation
////////////////////////////////////////////////////////////////////////////////

func (this *state) GetZoneReconcilation(logger logger.LogContext, zoneid dns.ZoneID) (time.Duration, bool, *zoneReconciliation) {
	req := &zoneReconciliation{
		fhandler: this.context,
	}

	this.lock.RLock()
	defer this.lock.RUnlock()

	hasProviders := this.hasProviders()
	zone := this.zones[zoneid]
	if zone == nil {
		return 0, hasProviders, nil
	}
	now := time.Now()
	req.zone = zone
	next := zone.GetNext()
	if now.Before(next) {
		return next.Sub(now), hasProviders, req
	}
	req.entries, req.equivEntries, req.stale, req.deleting = this.addEntriesForZone(logger, nil, nil, zone)
	req.providers = this.getProvidersForZone(zoneid)
	req.dnsTicker = this.dnsTicker
	return 0, hasProviders, req
}

func (this *state) reconcileZoneBlockingEntries(logger logger.LogContext) int {
	this.lock.RLock()
	defer this.lock.RUnlock()

	// remove long blockings to avoid blocking forever
	maxBlocking := 10 * time.Minute
	outdated := time.Now().Add(-1 * maxBlocking)
	for n, t := range this.blockingEntries {
		if t.Before(outdated) {
			// should never happen
			delete(this.blockingEntries, n)
			logger.Warnf("deleting blocking entry %s because blocked longer than %fm", n, maxBlocking.Minutes())
		}
	}
	return len(this.blockingEntries)
}

func (this *state) ReconcileZone(logger logger.LogContext, zoneid dns.ZoneID) reconcile.Status {
	logger.Infof("Initiate reconcilation of zone %s", zoneid)
	defer logger.Infof("zone %s done", zoneid)

	zoneDomain := this.getZoneDomain(zoneid)
	if zoneDomain == "" {
		logger.Warnf("zone %s not found -> stop reconciling", zoneid)
		return reconcile.Succeeded(logger, fmt.Sprintf("zone %s not found", zoneid))
	}

	blockingCount := this.reconcileZoneBlockingEntries(logger)
	if blockingCount > 0 {
		logger.Infof("reconciliation of zone %s is blocked due to %d pending entry reconciliations", zoneid, blockingCount)
		return reconcile.Succeeded(logger).RescheduleAfter(5 * time.Second)
	}

	startTime := time.Now()
	blockingEntries := this.getZoneEntries(zoneid)
	for i := 0; i < 50; i++ {
		blockingEntries = this.entriesLocking.TryLockZoneReconciliation(startTime, zoneid, zoneDomain, blockingEntries)
		if len(blockingEntries) == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
		if i%10 == 0 {
			logger.Infof("waiting for %d entries to be unlocked for zone %s", len(blockingEntries), zoneid)
		}
	}
	defer func() {
		cluster := this.context.GetCluster(TARGET_CLUSTER).GetId()
		entryGroupKind = resources.NewGroupKind(api.GroupName, api.DNSEntryKind)
		toBeTrigged := this.entriesLocking.UnlockZoneReconciliation(zoneid)
		for _, e := range toBeTrigged {
			logger.Infof("triggering entry %s for zone %s", e, zoneid)
			if err := this.context.EnqueueKey(resources.NewClusterKey(cluster, entryGroupKind, e.Namespace(), e.Name())); err != nil {
				logger.Errorf("failed to enqueue entry %s for zone %s: %s", e, zoneid, err)
			}
		}
	}()
	if len(blockingEntries) > 0 {
		logger.Infof("zone %s blocked by %d entries -> skip and reschedule", zoneid, len(blockingEntries))
		return reconcile.Succeeded(logger).RescheduleAfter(10 * time.Second)
	}

	delay, hasProviders, req := this.GetZoneReconcilation(logger, zoneid)
	if req == nil || req.zone == nil {
		if !hasProviders {
			return reconcile.Succeeded(logger).Stop()
		}
		return reconcile.Failed(logger, fmt.Errorf("zone %s not used anymore -> stop reconciling", zoneid))
	}
	logger = this.RefineLogger(logger, zoneid.ProviderType)
	if delay > 0 {
		logger.Infof("too early (required delay between two reconcilations: %s) -> skip and reschedule", this.config.Delay)
		return reconcile.Succeeded(logger).RescheduleAfter(delay)
	}
	logger.Infof("precondition fulfilled for zone %s", zoneid)
	if done, err := this.StartZoneReconcilation(logger, req); done {
		if err != nil {
			if _, ok := err.(*perrs.NoSuchHostedZone); ok {
				for _, provider := range req.providers {
					// trigger provider reconciliation to update its status
					_ = this.context.Enqueue(provider.Object())
				}
				return reconcile.Succeeded(logger)
			}
			logger.Infof("zone reconcilation failed for %s: %s", req.zone.Id(), err)
			return reconcile.Succeeded(logger).RescheduleAfter(req.zone.RateLimit())
		}
		if req.zone.nextTrigger > 0 {
			return reconcile.Succeeded(logger).RescheduleAfter(req.zone.nextTrigger)
		}
		return reconcile.Succeeded(logger)
	}
	logger.Infof("reconciling zone %q (%s) already busy and skipped", zoneid, req.zone.Domain())
	return reconcile.Succeeded(logger).RescheduleAfter(10 * time.Second)
}

func (this *state) getZoneDomain(id dns.ZoneID) string {
	this.lock.RLock()
	defer this.lock.RUnlock()

	zone := this.zones[id]
	if zone == nil {
		return ""
	}
	return zone.Domain()
}

func (this *state) getZoneEntries(id dns.ZoneID) []resources.ObjectName {
	this.lock.RLock()
	defer this.lock.RUnlock()

	var zoneEntries []resources.ObjectName
	for _, e := range this.entries {
		if e.ZoneId() == id {
			zoneEntries = append(zoneEntries, e.ObjectName())
		}
	}
	return zoneEntries
}

func (this *state) StartZoneReconcilation(logger logger.LogContext, req *zoneReconciliation) (bool, error) {
	if req.deleting {
		ctxutil.Tick(this.GetContext().GetContext(), controller.DeletionActivity)
	}
	if req.zone.TestAndSetBusy() {
		defer req.zone.Release()

		list := make(EntryList, 0, len(req.stale)+len(req.entries))
		for _, e := range req.entries {
			list = append(list, e)
		}
		for _, e := range req.stale {
			if req.entries[e.ObjectName()] == nil {
				list = append(list, e)
			} else {
				logger.Errorf("???, duplicate entry in stale and entries")
			}
		}
		logger.Infof("locking %d entries for zone reconcilation", len(list))
		if err := list.Lock(); err != nil {
			logger.Warnf("locking %d entries failed: %s", len(list), err)
			return false, err
		}
		defer func() {
			logger.Infof("unlocking %d entries", len(list))
			list.Unlock()
		}()
		return true, this.reconcileZone(logger, req)
	}
	return false, nil
}

func (this *state) reconcileZone(logger logger.LogContext, req *zoneReconciliation) error {
	zoneid := req.zone.Id()
	req.zone.SetNext(time.Now().Add(this.config.Delay))
	metrics.ReportZoneEntries(zoneid, len(req.entries), len(req.stale))
	logger.Infof("reconcile ZONE %s (%s) for %d dns entries (%d stale)", req.zone.Id(), req.zone.Domain(), len(req.entries), len(req.stale))

	current := this.startZoneTransaction(req.zone.Id())
	var oldDNSSets dns.DNSSets
	if current != nil {
		for _, x := range current.AllChanges() {
			logger.Infof("txn-change: %s", x)
		}
		oldDNSSets = current.OldDNSSets()
	}
	changes := NewChangeModel(logger, req, this.config, oldDNSSets)

	err := changes.Setup()
	if err != nil {
		req.zone.Failed()
		return err
	}
	req.zone.nextTrigger = 0
	modified := false
	var conflictErr error
	for _, e := range req.entries {
		// TODO: err handling
		var changeResult ChangeResult
		spec := e.object.GetTargetSpec(e)
		statusUpdate := NewStatusUpdate(logger, e, this.GetContext())
		if e.IsDeleting() {
			changeResult = changes.Delete(e.DNSSetName(), e.ObjectName().Namespace(), statusUpdate, spec)
		} else {
			if !e.NotRateLimited() {
				changeResult = changes.Check(e.DNSSetName(), e.ObjectName().Namespace(), statusUpdate, spec)
				if changeResult.Modified {
					if accepted, delay := this.tryAcceptProviderRateLimiter(logger, e); !accepted {
						req.zone.nextTrigger = delay
						changes.PseudoApply(e.DNSSetName(), spec)
						logger.Infof("rate limited %s, delay %.1f s", e.ObjectName(), delay.Seconds())
						statusUpdate.Throttled()
						if delay.Seconds() > 2 {
							e.object.Eventf(corev1.EventTypeNormal, "rate limit", "delayed for %1.fs", delay.Seconds())
						}
						continue
					}
				}
			}
			changeResult = changes.Apply(e.DNSSetName(), e.ObjectName().Namespace(), statusUpdate, spec)
			if changeResult.Error != nil && changeResult.Retry {
				conflictErr = changeResult.Error
			}
		}
		modified = modified || changeResult.Modified
	}
	modified = changes.Cleanup(logger) || modified
	if modified {
		err = changes.Update(logger)
	}

	outdatedEntries := EntryList{}
	this.outdated.AddActiveZoneTo(zoneid, &outdatedEntries)
	for _, e := range outdatedEntries {
		if changes.IsFailed(e.DNSSetName()) {
			if oldSet, ok := oldDNSSets[e.DNSSetName()]; ok {
				this.addDeleteToNextTransaction(logger, req, zoneid, e, oldSet)
			} else {
				logger.Warnf("cleanup postpone failure: old set not found for %s", e.ObjectName())
			}
			continue
		}
		if !changes.IsSucceeded(e.DNSSetName()) {
			// DNSEntry in deleting state, but not completely handled (e.g. after restart before zone reconciliation was running)
			if oldSet, ok := changes.zonestate.GetDNSSets()[e.DNSSetName()]; ok {
				this.addDeleteToNextTransaction(logger, req, zoneid, e, oldSet)
				continue
			}
		}
		logger.Infof("cleanup outdated entry %q", e.ObjectName())
		err := e.RemoveFinalizer()
		if err == nil || errors.IsNotFound(err) {
			this.outdated.Delete(e)
		}
	}
	if err == nil {
		req.zone.Succeeded()
		err = conflictErr
	} else {
		req.zone.Failed()
	}

	// give some time to reconcile rescheduled entries
	req.zone.SetNext(time.Now().Add(this.config.Delay))

	return err
}

func (this *state) addDeleteToNextTransaction(logger logger.LogContext, req *zoneReconciliation, zoneid dns.ZoneID, e *Entry, oldSet *dns.DNSSet) {
	if txn := this.getActiveZoneTransaction(zoneid); txn != nil {
		txn.AddEntryChange(e.ObjectKey(), e.Object().GetGeneration(), oldSet, nil)
		if req.zone.nextTrigger == 0 {
			req.zone.nextTrigger = this.config.Delay
		}
	} else {
		logger.Warnf("cleanup postpone failure: missing zone for %s", e.ObjectName())
	}
}

func (this *state) deleteZone(zoneid dns.ZoneID) {
	metrics.DeleteZone(zoneid)
	delete(this.zones, zoneid)
	this.triggerAllZonePolicies()
}

func (this *state) CreateStateTTLGetter(defaultStateTTL time.Duration) StateTTLGetter {
	return func(zoneid dns.ZoneID) time.Duration {
		if value := this.zoneStateTTL.Load(); value != nil {
			stateTTLMap := value.(map[dns.ZoneID]time.Duration)
			if ttl, ok := stateTTLMap[zoneid]; ok {
				return ttl
			}
		}
		return defaultStateTTL
	}
}

func (this *state) getActiveZoneTransaction(zoneID dns.ZoneID) *zonetxn.PendingTransaction {
	if zoneID.IsEmpty() {
		return nil
	}

	this.zoneTransactionsLock.Lock()
	defer this.zoneTransactionsLock.Unlock()

	current := this.zoneTransactions[zoneID]
	if current == nil {
		current = zonetxn.NewZoneTransaction(zoneID)
		this.zoneTransactions[zoneID] = current
	}
	return current
}

func (this *state) startZoneTransaction(zoneID dns.ZoneID) *zonetxn.PendingTransaction {
	if zoneID.IsEmpty() {
		return nil
	}

	this.zoneTransactionsLock.Lock()
	defer this.zoneTransactionsLock.Unlock()

	current := this.zoneTransactions[zoneID]
	delete(this.zoneTransactions, zoneID)
	return current
}
