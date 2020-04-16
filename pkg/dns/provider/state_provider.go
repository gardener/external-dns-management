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
	"github.com/gardener/controller-manager-library/pkg/utils"

	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	"github.com/gardener/external-dns-management/pkg/server/metrics"
)

////////////////////////////////////////////////////////////////////////////////
// provider handling
////////////////////////////////////////////////////////////////////////////////

func (this *state) addEntriesForProvider(p *dnsProviderVersion, entries Entries) {
	if p == nil {
		return
	}
	for n, e := range this.entries {
		name := e.DNSName()
		if name != "" && p.Match(e.DNSName()) > 0 {
			entries[n] = e
		}
	}
}
func (this *state) UpdateProvider(logger logger.LogContext, obj *dnsutils.DNSProviderObject) reconcile.Status {
	logger = this.RefineLogger(logger, obj.TypeCode())
	logger.Infof("reconcile PROVIDER")
	if !this.config.Enabled.Contains(obj.TypeCode()) || !this.config.Factory.IsResponsibleFor(obj) {
		return this._UpdateForeignProvider(logger, obj)
	}
	return this._UpdateLocalProvider(logger, obj)
}

func (this *state) _UpdateLocalProvider(logger logger.LogContext, obj *dnsutils.DNSProviderObject) reconcile.Status {
	err := this.SetFinalizer(obj)
	if err != nil {
		return reconcile.Delay(logger, fmt.Errorf("cannot set finalizer: %s", err))
	}

	p := this.GetProvider(obj.ObjectName())

	var last *dnsProviderVersion
	if p != nil {
		last = p.(*dnsProviderVersion)
	}

	new, status := updateDNSProvider(logger, this, obj, last)

	if last != nil && last.account != nil && last.account != new.account {
		this.accountCache.Release(logger, last.account, obj.ObjectName())
	}

	this.lock.Lock()
	defer this.lock.Unlock()
	regmod, regerr := this.registerSecret(logger, new.secret, new)

	this.providers[new.ObjectName()] = new
	mod := this.updateZones(logger, last, new)
	if !status.IsSucceeded() {
		logger.Infof("errorneous provider: %s", status.Error)
		if last != nil {
			logger.Infof("trigger old zones")
			for _, z := range last.zones {
				this.triggerHostedZone(z.Id())
			}
		}
		if regmod {
			return reconcile.Repeat(logger, regerr)
		}
		return status
	}

	if regerr != nil {
		status = reconcile.Delay(logger, regerr)
	}

	entries := Entries{}
	if last == nil || !new.equivalentTo(last) {
		this.addEntriesForProvider(last, entries)
		this.addEntriesForProvider(new, entries)
	}

	if mod || (last != nil && !last.IsValid() && new.IsValid()) {
		logger.Infof("found %d zones: ", len(new.zones))
		for _, z := range new.zones {
			logger.Infof("    %s: %s", z.Id(), z.Domain())
			if len(z.ForwardedDomains()) > 0 {
				logger.Infof("        forwarded: %s", utils.Strings(z.ForwardedDomains()...))
			}
		}
		this.addBlockingEntries(logger, entries)
		this.TriggerEntries(logger, entries)
	}
	if last != nil && !last.IsValid() && new.IsValid() {
		logger.Infof("trigger new zones for repaired provider")
		for _, z := range new.zones {
			this.triggerHostedZone(z.Id())
		}
	}
	return status
}

func (this *state) _UpdateForeignProvider(logger logger.LogContext, obj *dnsutils.DNSProviderObject) reconcile.Status {
	pname := obj.ObjectName()

	this.lock.Lock()
	defer this.lock.Unlock()

	if this.providers[pname] != nil {
		logger.Infof("provider %q switched type to %q -> remove it", obj.ObjectName(), obj.DNSProvider().Spec.Type)
		this.removeLocalProvider(logger, obj)
	}

	cur := this.foreign[pname]
	if cur == nil {
		cur = newForeignProvider(pname)
		this.foreign[pname] = cur
	}
	return cur.Update(logger, obj).StopIfSucceeded()
}

func (this *state) removeForeignProvider(logger logger.LogContext, pname resources.ObjectName) reconcile.Status {
	foreign := this.foreign[pname]
	if foreign != nil {
		logger.Infof("removing foreign provider %q", pname)
		delete(this.foreign, pname)
	}
	return reconcile.Succeeded(logger)
}

func (this *state) ProviderDeleted(logger logger.LogContext, key resources.ObjectKey) reconcile.Status {

	this.lock.Lock()
	defer this.lock.Unlock()

	return this.removeForeignProvider(logger, key.ObjectName())
}

func (this *state) RemoveProvider(logger logger.LogContext, obj *dnsutils.DNSProviderObject) reconcile.Status {
	pname := obj.ObjectName()

	this.lock.Lock()
	defer this.lock.Unlock()

	foreign := this.foreign[pname]
	if foreign != nil {
		return this.removeForeignProvider(logger, pname)
	}
	return this.removeLocalProvider(logger, obj)
}

func (this *state) removeLocalProvider(logger logger.LogContext, obj *dnsutils.DNSProviderObject) reconcile.Status {
	pname := obj.ObjectName()
	cur := this.providers[pname]
	if cur != nil {
		this.deleting[pname] = cur
		delete(this.providers, pname)
	} else {
		cur = this.deleting[pname]
	}
	if cur != nil {
		entries := Entries{}
		zones := this.providerzones[obj.ObjectName()]
		logger.Infof("deleting PROVIDER with %d zones", len(zones))
		for n, z := range zones {
			if this.isProviderForZone(n, pname) {
				this.addEntriesForZone(logger, entries, nil, z)
				providers := this.getProvidersForZone(n)
				if len(providers) == 1 {
					// if this is the last provider for this zone
					// it must be cleanuped before the provider is gone
					logger.Infof("provider is exclusively handling zone %q -> cleanup", n)

					done, err := this.StartZoneReconcilation(logger, &zoneReconciliation{
						zone:      z,
						providers: providers,
						entries:   Entries{},
						stale:     nil,
						dedicated: false,
						deleting:  false,
					})
					if !done {
						return reconcile.Delay(logger, fmt.Errorf("zone reconcilation busy -> delay deletion"))
					}
					if err != nil {
						if _, ok := err.(*perrs.NoSuchHostedZone); !ok {
							logger.Errorf("zone cleanup failed: %s", err)
							return reconcile.Delay(logger, fmt.Errorf("zone reconcilation failed -> delay deletion"))
						}
					}
					metrics.DeleteZone(n)
					delete(this.zones, n)
				} else {
					// delete entries in hosted zone exclusively covered by this provider using
					// other provider for this zone
					logger.Infof("delegate zone cleanup of %q to other provider", n)
					this.triggerHostedZone(n)
				}
				this.removeProviderForZone(n, pname)
			} else {
				logger.Infof("not reponsible for zone %q", n)
			}
		}
		logger.Infof("zone cleanup done -> trigger entries")
		this.TriggerEntries(logger, this.entries)
		logger.Infof("releasing provider secret")
		_, err := this.registerSecret(logger, nil, cur)
		if err != nil {
			return reconcile.Delay(logger, err)
		}
		logger.Infof("releasing account cache")
		this.accountCache.Release(logger, cur.account, cur.ObjectName())
		delete(this.deleting, obj.ObjectName())
		delete(this.providerzones, obj.ObjectName())
		logger.Infof("finally remove finalizer")
		return reconcile.DelayOnError(logger, this.RemoveFinalizer(cur.Object()))
	}
	return reconcile.Succeeded(logger)
}
