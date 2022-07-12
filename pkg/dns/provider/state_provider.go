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
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"k8s.io/client-go/util/flowcontrol"

	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
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
		this.informProviderRemoved(logger, new.ObjectName())
		logger.Infof("errorneous provider: %s", status.Error)
		if last != nil {
			logger.Infof("trigger entries for old zones")
			entries := Entries{}
			stale := RecordSetNames{}
			for _, z := range last.zones {
				this.addEntriesForZone(logger, entries, stale, z)
			}
			for _, s := range stale {
				entries.AddEntry(s)
			}
			this.TriggerEntries(logger, entries)
		}
		if regmod {
			return reconcile.Repeat(logger, regerr)
		}
		return status
	}

	this.informProviderUpdated(logger, new)

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
	}
	if len(entries) > 0 && (mod || new.IsValid()) {
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

func (this *state) updateProviderRateLimiter(logger logger.LogContext, obj *dnsutils.DNSProviderObject) *api.RateLimit {
	this.prlock.Lock()
	defer this.prlock.Unlock()

	rateLimit := obj.Spec().RateLimit
	if rateLimit != nil {
		data, ok := this.providerRateLimiter[obj.ObjectName()]
		if !ok || data.RateLimit.RequestsPerDay != rateLimit.RequestsPerDay || data.RateLimit.Burst != rateLimit.Burst {
			qps := float32(rateLimit.RequestsPerDay) / 86400
			data = &rateLimiterData{
				RateLimit:   *rateLimit,
				rateLimiter: flowcontrol.NewTokenBucketRateLimiter(qps, rateLimit.Burst),
			}
			this.providerRateLimiter[obj.ObjectName()] = data
			logger.Infof("frontend rate limiter updated: requestsPerDay=%d, burst=%d", rateLimit.RequestsPerDay, rateLimit.Burst)
		}
	} else {
		if _, ok := this.providerRateLimiter[obj.ObjectName()]; ok {
			delete(this.providerRateLimiter, obj.ObjectName())
			logger.Infof("frontend rate limiter deleted")
		}
	}
	return rateLimit
}

func (this *state) informProviderUpdated(logger logger.LogContext, new *dnsProviderVersion) {
	for _, listener := range this.providerEventListeners {
		listener.ProviderUpdatedEvent(logger, new.ObjectName(), new.Object().GetAnnotations(), handler(new))
	}
}

func (this *state) informProviderRemoved(logger logger.LogContext, name resources.ObjectName) {
	for _, listener := range this.providerEventListeners {
		listener.ProviderRemovedEvent(logger, name)
	}
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

	this.informProviderRemoved(logger, key.ObjectName())
	return this.removeForeignProvider(logger, key.ObjectName())
}

func (this *state) RemoveProvider(logger logger.LogContext, obj *dnsutils.DNSProviderObject) reconcile.Status {
	this.informProviderRemoved(logger, obj.ObjectName())

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
		zones := this.providerzones[obj.ObjectName()]
		logger.Infof("deleting PROVIDER with %d zones", len(zones))
		for zoneid, z := range zones {
			if this.isProviderForZone(zoneid, pname) {
				providers := this.getProvidersForZone(zoneid)
				if len(providers) == 1 {
					// if this is the last provider for this zone
					// it must be cleanuped before the provider is gone
					logger.Infof("provider is exclusively handling zone %q -> cleanup", zoneid)

					done, err := this.StartZoneReconcilation(logger, &zoneReconciliation{
						zone:      z,
						providers: providers,
						entries:   Entries{},
						stale:     nil,
						dedicated: false,
						deleting:  false,
						fhandler:  this.context,
						ownership: this.ownerCache,
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
					this.deleteZone(zoneid)
				} else {
					// delete entries in hosted zone exclusively covered by this provider using
					// other provider for this zone
					logger.Infof("delegate zone cleanup of %q to other provider", zoneid)
					this.triggerHostedZone(zoneid)
				}
				this.removeProviderForZone(zoneid, pname)
			} else {
				logger.Infof("not reponsible for zone %q", zoneid)
			}
		}
		logger.Infof("zone cleanup done -> trigger entries")
		for _, e := range this.entries {
			if e.providername == pname {
				this.TriggerEntry(logger, e)
			}
		}
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
