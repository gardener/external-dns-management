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
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
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
	}
	return reconcile.Succeeded(logger)
}
