// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns/provider/statistic"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	"github.com/gardener/external-dns-management/pkg/server/metrics"
)

////////////////////////////////////////////////////////////////////////////////
// state handling for OwnerIds
////////////////////////////////////////////////////////////////////////////////

func delta(changed, active utils.StringSet) string {
	msg := ""
	added := utils.NewStringSet()
	deleted := utils.NewStringSet()
	for k := range changed {
		if active.Contains(k) {
			added.Add(k)
		} else {
			deleted.Add(k)
		}
	}
	s := ""
	if len(added) > 0 {
		s = fmt.Sprintf(" added: %s%s", added, msg)
	}
	if len(deleted) > 0 {
		s = fmt.Sprintf("%s deleted: %s%s", s, deleted, msg)
	}
	if s == "" {
		return fmt.Sprintf("no change%s", msg)
	}
	return s[1:]
}

func (this *state) UpdateOwner(logger logger.LogContext, owner *dnsutils.DNSOwnerObject, setup bool) reconcile.Status {
	if !setup && !this.ownerCache.IsResponsibleFor(owner.GetOwnerId()) && owner.IsActive() {
		logger.Infof("would activate new owner -> ensure all entries are synchronized")
		this.ownerCache.SetPending(owner.GetOwnerId())
		done, err := this.context.Synchronize(logger, SYNC_ENTRIES, owner.Object)
		if !done || err != nil {
			return reconcile.DelayOnError(logger, err)
		}
		logger.Infof("entries synchronized")
	}
	this.lock.Lock()
	changed, active := this.ownerCache.UpdateOwner(owner)
	this.lock.Unlock()
	logger.Infof("update: owner ids %s", delta(changed, active))
	logger.Debugf("       active owner ids %s", active)
	if len(changed) > 0 {
		this.TriggerEntriesByOwner(logger, changed)
		this.TriggerHostedZonesByChangedOwners(logger, changed)
	}
	if statusActive := owner.Status().Active; statusActive == nil || *statusActive != owner.IsActive() {
		isActive := owner.IsActive()
		owner.Status().Active = &isActive
		err := owner.UpdateStatus()
		if err != nil {
			return reconcile.DelayOnError(logger, fmt.Errorf("cannot update status of %s: %w", owner.ObjectName(), err))
		}
	}
	return reconcile.Succeeded(logger)
}

func (this *state) OwnerDeleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	this.lock.Lock()
	changed, active := this.ownerCache.DeleteOwner(key)
	this.lock.Unlock()
	logger.Infof("delete: changed owner ids %s", changed)
	logger.Debugf("       active owner ids %s", active)
	if len(changed) > 0 {
		this.TriggerEntriesByOwner(logger, changed)
		this.TriggerHostedZonesByChangedOwners(logger, changed)
	}
	return reconcile.Succeeded(logger)
}

func (this *state) UpdateOwnerCounts(log logger.LogContext) {
	if !this.initialized {
		return
	}
	log.Infof("update owner statistic")
	statistic := statistic.NewEntryStatistic()
	this.UpdateStatistic(statistic)
	types := this.GetHandlerFactory().TypeCodes()
	metrics.UpdateOwnerStatistic(statistic, types)
	changes := this.ownerCache.UpdateCountsWith(statistic.Owners, types)
	if len(changes) > 0 {
		log.Infof("found %d changes for owner usages", len(changes))
		this.ownerupd <- changes
	}
}

////////////////////////////////////////////////////////////////////////////////

func startOwnerUpdater(pctx ProviderContext, ownerresc resources.Interface) chan OwnerCounts {
	log := pctx.AddIndent("updater: ")

	requests := make(chan OwnerCounts, 2)
	go func() {
		log.Infof("starting owner count updater")
		for {
			select {
			case <-pctx.GetContext().Done():
				log.Infof("stopping owner updater")
				return
			case changes := <-requests:
				log.Infof("starting owner update for %d changes", len(changes))
				for n, counts := range changes {
					log.Infof("  updating owner counts %v for %s", counts, n)
					_, _, err := ownerresc.ModifyStatusByName(resources.NewObjectName(string(n)), func(data resources.ObjectData) (bool, error) {
						owner, ok := data.(*v1alpha1.DNSOwner)
						if !ok {
							return false, fmt.Errorf("invalid owner object type %T", data)
						}
						mod := false
						if owner.Status.Entries.ByType == nil {
							owner.Status.Entries.ByType = ProviderTypeCounts{}
						}
						for t, v := range counts {
							if owner.Status.Entries.ByType[t] != v {
								mod = true
								owner.Status.Entries.ByType[t] = v
							}
							if v == 0 {
								delete(owner.Status.Entries.ByType, t)
							}
						}
						sum := 0
						for _, v := range owner.Status.Entries.ByType {
							sum += v
						}
						if owner.Status.Entries.Amount != sum {
							owner.Status.Entries.Amount = sum
							mod = true
						}
						return mod, nil
					})
					if err != nil {
						log.Errorf("update failed: %s", err)
					}
				}
			}
		}
	}()
	return requests
}
