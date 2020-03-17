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

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns/provider/statistic"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	"github.com/gardener/external-dns-management/pkg/server/metrics"
)

////////////////////////////////////////////////////////////////////////////////
// state handling for OwnerIds
////////////////////////////////////////////////////////////////////////////////

func (this *state) UpdateOwner(logger logger.LogContext, owner *dnsutils.DNSOwnerObject, setup bool) reconcile.Status {
	if !setup && !this.ownerCache.IsResponsibleFor(owner.GetOwnerId()) && owner.IsActive() {
		logger.Infof("would activate new owner -> ensure all entries are synchronized")
		done, err:=this.context.Synchronize(logger,SYNC_ENTRIES, owner.Object)
		if !done || err != nil {
		    return reconcile.DelayOnError(logger,err)
		}
		logger.Infof("entries synchronized")
	}
	changed, active := this.ownerCache.UpdateOwner(owner)
	logger.Infof("update: changed owner ids %s, active owner ids %s", changed, active)
	if len(changed) > 0 {
		this.TriggerEntriesByOwner(logger, changed)
		this.TriggerHostedZones()
	}
	return reconcile.Succeeded(logger)
}

func (this *state) OwnerDeleted(logger logger.LogContext, key resources.ObjectKey) reconcile.Status {
	changed, active := this.ownerCache.DeleteOwner(key)
	logger.Infof("delete: changed owner ids %s, active owner ids %s", changed, active)
	if len(changed) > 0 {
		this.TriggerEntriesByOwner(logger, changed)
		this.TriggerHostedZones()
	}
	return reconcile.Succeeded(logger)
}

func (this *state) UpdateOwnerCounts(log logger.LogContext) {
	if !this.initialized {
		return
	}
	statistic := statistic.NewEntryStatistic()
	this.updateStatistics(statistic)
	types := this.GetHandlerFactory().TypeCodes()
	metrics.UpdateOwnerStatistic(statistic, types)
	changes := this.ownerCache.UpdateCountsWith(statistic.Owners, types)
	if len(changes) > 0 {
		log.Infof("found %d changes for owner usages", len(changes))
		this.ownerupd <- changes
	}
}

////////////////////////////////////////////////////////////////////////////////

func startOwnerUpdater(ctx Context, ownerresc resources.Interface) chan OwnerCounts {
	log := ctx.AddIndent("updater: ")

	requests := make(chan OwnerCounts, 2)
	go func() {
		log.Infof("starting owner count updater")
		for {
			select {
			case <-ctx.GetContext().Done():
				log.Infof("stopping owner updater")
				return
			case changes := <-requests:
				log.Infof("starting owner update for %d changes", len(changes))
				for n, counts := range changes {
					log.Infof("  updating owner counts %v for %s", counts, n)
					_, _, err := ownerresc.ModifyStatusByName(resources.NewObjectName(n), func(data resources.ObjectData) (bool, error) {
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
