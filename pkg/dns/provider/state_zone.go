/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package provider

import (
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"

	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
	"github.com/gardener/external-dns-management/pkg/server/metrics"
)

////////////////////////////////////////////////////////////////////////////////
// state handling for zone reconcilation
////////////////////////////////////////////////////////////////////////////////

func (this *state) TriggerHostedZone(name string) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.triggerHostedZone(name)
}

func (this *state) TriggerHostedZonesByChangedOwners(logger logger.LogContext, changed utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()
	for zoneid, zone := range this.zones {
		if intersection := zone.IntersectOwners(changed); len(intersection) > 0 {
			logger.Infof("trigger zone %s because of changed owners %s", zoneid, intersection)
			this.triggerHostedZone(zoneid)
		}
	}
}

func (this *state) GetZoneReconcilation(logger logger.LogContext, zoneid string) (time.Duration, bool, *zoneReconciliation) {
	req := &zoneReconciliation{
		fhandler: this.context,
	}

	this.lock.RLock()
	defer this.lock.RUnlock()

	req.ownership = this.ownerCache
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
	req.entries, req.stale, req.deleting = this.addEntriesForZone(logger, nil, nil, zone)
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

func (this *state) ReconcileZone(logger logger.LogContext, zoneid string) reconcile.Status {
	logger.Infof("Initiate reconcilation of zone %s", zoneid)
	defer logger.Infof("zone %s done", zoneid)

	blockingCount := this.reconcileZoneBlockingEntries(logger)
	if blockingCount > 0 {
		logger.Infof("reconciliation of zone %s is blocked due to %d pending entry reconciliations", zoneid, blockingCount)
		return reconcile.Succeeded(logger).RescheduleAfter(5 * time.Second)
	}

	delay, hasProviders, req := this.GetZoneReconcilation(logger, zoneid)
	if req == nil || req.zone == nil {
		if !hasProviders {
			return reconcile.Succeeded(logger).Stop()
		}
		return reconcile.Failed(logger, fmt.Errorf("zone %s not used anymore -> stop reconciling", zoneid))
	}
	logger = this.RefineLogger(logger, req.zone.ProviderType())
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
		return reconcile.Succeeded(logger)
	}
	logger.Infof("reconciling zone %q (%s) already busy and skipped", zoneid, req.zone.Domain())
	return reconcile.Succeeded(logger).RescheduleAfter(10 * time.Second)
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
			this.triggerStatistic()
		}()
		return true, this.reconcileZone(logger, req)
	}
	return false, nil
}

func (this *state) reconcileZone(logger logger.LogContext, req *zoneReconciliation) error {
	zoneid := req.zone.Id()
	req.zone.SetNext(time.Now().Add(this.config.Delay))
	metrics.ReportZoneEntries(req.zone.ProviderType(), zoneid, len(req.entries), len(req.stale))
	logger.Infof("reconcile ZONE %s (%s) for %d dns entries (%d stale)", req.zone.Id(), req.zone.Domain(), len(req.entries), len(req.stale))
	logger.Debugf("    ownerids: %s", req.ownership.GetIds())
	changes := NewChangeModel(logger, req.ownership, req, this.config)
	err := changes.Setup()
	if err != nil {
		req.zone.Failed()
		return err
	}
	modified := false
	var conflictErr error
	for _, e := range req.entries {
		// TODO: err handling
		var changeResult ChangeResult
		if e.IsDeleting() {
			changeResult = changes.Delete(e.DNSName(), e.ObjectName().Namespace(), e.CreatedAt(), NewStatusUpdate(logger, e, this.GetContext()), e.Kind())
		} else {
			changeResult = changes.Apply(e.DNSName(), e.ObjectName().Namespace(), e.CreatedAt(), NewStatusUpdate(logger, e, this.GetContext()), e.Kind(), e.Targets()...)
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
		if changes.IsFailed(e.DNSName()) {
			continue
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
	return err
}

func (this *state) deleteZone(zoneid string) {
	metrics.DeleteZone(zoneid)
	delete(this.zones, zoneid)
	this.triggerAllZonePolicies()
}

func (this *state) CreateStateTTLGetter(defaultStateTTL time.Duration) StateTTLGetter {
	return func(zoneid string) time.Duration {
		if value := this.zoneStateTTL.Load(); value != nil {
			stateTTLMap := value.(map[string]time.Duration)
			if ttl, ok := stateTTLMap[zoneid]; ok {
				return ttl
			}
		}
		return defaultStateTTL
	}
}
