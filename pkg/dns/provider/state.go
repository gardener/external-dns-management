/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package provider

import (
	"fmt"
	"github.com/gardener/external-dns-management/pkg/server/metrics"
	"k8s.io/apimachinery/pkg/runtime"
	"strings"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/utils"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
)

type DNSNames map[string]*Entry

type state struct {
	lock       sync.RWMutex
	classes    *dnsutils.Classes
	controller controller.Interface
	config     Config

	pending     utils.StringSet
	pendingKeys resources.ClusterObjectKeySet

	accountCache *AccountCache
	ownerCache   *OwnerCache

	foreign         map[resources.ObjectName]*foreignProvider
	providers       map[resources.ObjectName]*dnsProviderVersion
	deleting        map[resources.ObjectName]*dnsProviderVersion
	secrets         map[resources.ObjectName]resources.ObjectNameSet
	zones           map[string]*dnsHostedZone
	zoneproviders   map[string]resources.ObjectNameSet
	providerzones   map[resources.ObjectName]map[string]*dnsHostedZone
	providersecrets map[resources.ObjectName]resources.ObjectName

	entries  Entries
	outdated Entries

	dnsnames DNSNames

	initialized bool
}

func NewDNSState(controller controller.Interface, classes *dnsutils.Classes, config Config) *state {
	controller.Infof("responsible for classes: %s (%s)", classes, classes.Main())
	controller.Infof("using default ttl:       %d", config.TTL)
	controller.Infof("using identifier:        %s", config.Ident)
	controller.Infof("dry run mode:            %t", config.Dryrun)
	return &state{
		classes:         classes,
		controller:      controller,
		config:          config,
		accountCache:    NewAccountCache(config.CacheTTL),
		ownerCache:      NewOwnerCache(&config),
		pending:         utils.StringSet{},
		pendingKeys:     resources.ClusterObjectKeySet{},
		foreign:         map[resources.ObjectName]*foreignProvider{},
		providers:       map[resources.ObjectName]*dnsProviderVersion{},
		deleting:        map[resources.ObjectName]*dnsProviderVersion{},
		zones:           map[string]*dnsHostedZone{},
		secrets:         map[resources.ObjectName]resources.ObjectNameSet{},
		zoneproviders:   map[string]resources.ObjectNameSet{},
		providerzones:   map[resources.ObjectName]map[string]*dnsHostedZone{},
		providersecrets: map[resources.ObjectName]resources.ObjectName{},
		entries:         Entries{},
		outdated:        Entries{},
		dnsnames:        map[string]*Entry{},
	}
}

func (this *state) IsResponsibleFor(logger logger.LogContext, obj resources.Object) bool {
	return this.classes.IsResponsibleFor(logger, obj)
}

func (this *state) Setup() {
	processors, err := this.controller.GetIntOption(OPT_SETUP)
	if err != nil || processors <= 0 {
		processors = 5
	}
	this.controller.Infof("using %d parallel workers for initialization", processors)
	this.setupFor(&api.DNSProvider{}, "providers", func(e resources.Object) {
		p := dnsutils.DNSProvider(e)
		if this.GetHandlerFactory().IsResponsibleFor(p) {
			this.UpdateProvider(this.controller.NewContext("provider", p.ObjectName().String()), p)
		}
	}, processors)
	this.setupFor(&api.DNSOwner{}, "owners", func(e resources.Object) {
		p := dnsutils.DNSOwner(e)
		this.UpdateOwner(this.controller.NewContext("owner", p.ObjectName().String()), p)
	}, processors)
	this.setupFor(&api.DNSEntry{}, "entries", func(e resources.Object) {
		p := dnsutils.DNSEntry(e)
		this.UpdateEntry(this.controller.NewContext("entry", p.ObjectName().String()), p)
	}, processors)

	this.initialized = true
	this.controller.Infof("setup done - starting reconcilation")
}

func (this *state) setupFor(obj runtime.Object, msg string, exec func(resources.Object), processors int) {
	this.controller.Infof("### setup %s", msg)
	res, _ := this.controller.GetMainCluster().Resources().GetByExample(obj)
	list, _ := res.ListCached(labels.Everything())
	dnsutils.ProcessElements(list, func(e resources.Object) {
		if this.IsResponsibleFor(this.controller, e) {
			exec(e)
		}
	}, processors)
}

func (this *state) Start() {
	for c := range this.pending {
		this.controller.Infof("trigger %s", c)
		this.controller.EnqueueCommand(c)
	}

	for key := range this.pendingKeys {
		this.controller.Infof("trigger key %s/%s", key.Namespace(), key.Name())
		this.controller.EnqueueKey(key)
		delete(this.pendingKeys, key)
	}
}

func (this *state) HasFinalizer(obj resources.Object) bool {
	return this.GetController().HasFinalizer(obj)
}

func (this *state) SetFinalizer(obj resources.Object) error {
	return this.GetController().SetFinalizer(obj)
}

func (this *state) RemoveFinalizer(obj resources.Object) error {
	return this.GetController().RemoveFinalizer(obj)
}

func (this *state) GetController() controller.Interface {
	return this.controller
}

func (this *state) GetConfig() Config {
	return this.config
}

func (this *state) GetDNSAccount(logger logger.LogContext, provider *dnsutils.DNSProviderObject, props utils.Properties) (*DNSAccount, error) {
	return this.accountCache.Get(logger, provider, props, this)
}

func (this *state) GetHandlerFactory() DNSHandlerFactory {
	return this.config.Factory
}

func (this *state) GetProvidersForZone(zoneid string) DNSProviders {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.getProvidersForZone(zoneid)
}

func (this *state) HasProvidersForZone(zoneid string) bool {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.hasProvidersForZone(zoneid)
}

func (this *state) hasProvidersForZone(zoneid string) bool {
	return len(this.zoneproviders[zoneid]) > 0
}

func (this *state) isProviderForZone(zoneid string, p resources.ObjectName) bool {
	set := this.zoneproviders[zoneid]
	return set != nil && set.Contains(p)
}

func (this *state) getProvidersForZone(zoneid string) DNSProviders {
	result := DNSProviders{}
	for n := range this.zoneproviders[zoneid] {
		p := this.providers[n]
		if p == nil {
			p = this.deleting[n]
			if p == nil {
				panic(fmt.Sprintf("OOPS: invalid provider %q for zone %q", n, zoneid))
			}
		}
		result[n] = p
	}
	return result
}

func (this *state) addProviderForZone(zoneid string, p resources.ObjectName) {
	set := this.zoneproviders[zoneid]
	if set == nil {
		set = resources.ObjectNameSet{}
		this.zoneproviders[zoneid] = set
	}
	set.Add(p)
}

func (this *state) removeProviderForZone(zoneid string, p resources.ObjectName) {
	set := this.zoneproviders[zoneid]
	if set != nil {
		set.Remove(p)
		if len(set) == 0 {
			delete(this.zoneproviders, zoneid)
		}
	}
}

func (this *state) LookupProvider(e *Entry) (DNSProvider, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.lookupProvider(e)
}

func (this *state) lookupProvider(e *Entry) (DNSProvider, error) {
	var err error
	var found DNSProvider
	match := -1
	for _, p := range this.providers {
		if p.IsValid() {
			n := p.Match(e.DNSName())
			if n > 0 {
				if match < n {
					err = CheckAccess(e.Object(), p.Object())
					if err == nil {
						found = p
					}
				}
			}
		}
	}
	return found, err
}

func (this *state) GetSecretUsage(name resources.ObjectName) []resources.Object {
	this.lock.RLock()
	defer this.lock.RUnlock()

	set := this.secrets[name]
	if set == nil {
		return []resources.Object{}
	}
	result := make([]resources.Object, 0, len(set))
	for n := range set {
		o := this.providers[n]
		result = append(result, o.object)
	}
	return result
}

func (this *state) registerSecret(logger logger.LogContext, secret resources.ObjectName, provider *dnsProviderVersion) error {
	pname := provider.ObjectName()
	old := this.providersecrets[pname]

	if old != nil && old != secret {
		oldp := this.secrets[old]
		if oldp.Contains(pname) {
			logger.Infof("releasing secret %q for provider %q", old, pname)
			if len(oldp) <= 1 {
				r, err := provider.Object().Resources().Get(&corev1.Secret{})
				s, err := r.GetCached(old)
				if err != nil {
					if !errors.IsNotFound(err) {
						logger.Warnf("cannot release secret %q for provider %q: %s", old, pname, err)
						return err
					}
				} else {
					logger.Infof("remove finalizer for unused secret %q", old)
					err := this.RemoveFinalizer(s)
					if err != nil && !errors.IsNotFound(err) {
						logger.Warnf("cannot release secret %q for provider %q: 5s", old, pname, err)
						return err
					}
				}
				delete(this.secrets, old)
			} else {
				delete(oldp, pname)
			}
		}
	}
	if secret != nil {
		if old != secret {
			logger.Infof("registering secret %q for provider %q", secret, pname)
			this.providersecrets[pname] = secret

			curp := this.secrets[secret]
			if curp == nil {
				curp = resources.ObjectNameSet{}
				this.secrets[secret] = curp
			}
			curp.Add(pname)
		}

		r, err := provider.Object().Resources().Get(&corev1.Secret{})
		s, err := r.GetCached(secret)
		if err == nil {
			err = this.SetFinalizer(s)
		}
		if err != nil {
			if errors.IsNotFound(err) {
				return fmt.Errorf("secret %q for provider %q not found", secret, pname)
			} else {
				return fmt.Errorf("cannot set finalizer for secret %q for provider %q: %s", secret, pname, err)
			}
		}
	}
	return nil
}

func (this *state) GetProvider(name resources.ObjectName) DNSProvider {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.providers[name]
}

func (this *state) GetZonesForProvider(name resources.ObjectName) dnsHostedZones {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return copyZones(this.providerzones[name])
}

func (this *state) GetEntriesForZone(logger logger.LogContext, zoneid string) (Entries, DNSNames) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	entries := Entries{}
	zone := this.zones[zoneid]
	if zone != nil {
		return this.addEntriesForZone(logger, entries, DNSNames{}, zone)
	}
	return entries, nil
}

func (this *state) addEntriesForZone(logger logger.LogContext, entries Entries, stale DNSNames, zone *dnsHostedZone) (Entries, DNSNames) {
	if entries == nil {
		entries = Entries{}
	}
	if stale == nil {
		stale = DNSNames{}
	}
	domain := zone.Domain()
	nested := utils.NewStringSet(zone.ForwardedDomains()...)
	for _, z := range this.zones {
		if z.Domain() != domain && dnsutils.Match(z.Domain(), domain) {
			nested.Add(z.Domain())
		}
	}
loop:
	for dns, e := range this.dnsnames {
		if e.IsValid() {
			provider, err := this.lookupProvider(e)
			if provider == nil && !e.IsDeleting() {
				if err != nil {
					logger.Infof("no valid provider found for %q(%s): %s", e.ObjectName(), dns, err)
				} else {
					logger.Infof("no valid provider found for %q(%s)", e.ObjectName(), dns)
				}
				stale[e.DNSName()] = e
				continue
			}
			if dnsutils.Match(dns, domain) {
				for _, excl := range zone.ForwardedDomains() {
					if dnsutils.Match(dns, excl) {
						continue loop
					}
				}
				for excl := range nested { // fallback if no forwarded domains are reported
					if dnsutils.Match(dns, excl) {
						continue loop
					}
				}
				if e.IsActive() {
					entries[e.ObjectName()] = e
				} else {
					logger.Infof("entry %q(%s) is inactive", e.ObjectName(), e.DNSName())
				}
			}
		} else {
			if !e.IsDeleting() {
				logger.Infof("invalid entry %q (%s)", e.ObjectName(), e.DNSName())
				stale[e.DNSName()] = e
			}
		}
	}
	return entries, stale
}

func (this *state) GetZoneForEntry(e *Entry) string {
	if !e.IsValid() {
		return ""
	}
	zoneid, _ := this.GetZoneForName(e.DNSName())
	return zoneid
}

func (this *state) GetZoneForName(name string) (string, int) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.getZoneForName(name)
}

func (this *state) getZoneForName(hostname string) (string, int) {
	length := 0
	found := ""
loop:
	for zoneid, zone := range this.zones {
		name := zone.Domain()
		if dnsutils.Match(hostname, name) {
			for _, f := range zone.ForwardedDomains() {
				if dnsutils.Match(hostname, f) {
					continue loop
				}
			}
			if length < len(name) {
				length = len(name)
				found = zoneid
			}
		}
	}
	return found, length
}

func (this *state) triggerHostedZone(name string) {
	cmd := HOSTEDZONE_PREFIX + name
	if this.controller.IsReady() {
		this.controller.EnqueueCommand(cmd)
	} else {
		this.pending.Add(cmd)
	}
}

func (this *state) triggerKey(key resources.ClusterObjectKey) {
	if this.controller.IsReady() {
		this.controller.EnqueueKey(key)
	} else {
		this.pendingKeys.Add(key)
	}
}

func (this *state) DecodeZoneCommand(name string) string {
	if strings.HasPrefix(name, HOSTEDZONE_PREFIX) {
		return name[len(HOSTEDZONE_PREFIX):]
	}
	return ""
}

func (this *state) updateZones(logger logger.LogContext, last, new *dnsProviderVersion) bool {
	var name resources.ObjectName
	keeping := []string{}
	modified := false
	result := map[string]*dnsHostedZone{}
	if new != nil {
		name = new.ObjectName()
		for _, z := range new.zones {
			zone := this.zones[z.Id()]
			if zone == nil {
				modified = true
				zone = newDNSHostedZone(z)
				this.zones[z.Id()] = zone
				logger.Infof("adding hosted zone %q (%s)", z.Id(), z.Domain())
				this.triggerHostedZone(zone.Id())
			}
			zone.update(z)

			if this.isProviderForZone(z.Id(), name) {
				keeping = append(keeping, fmt.Sprintf("keeping provider %q for hosted zone %q (%s)", name, z.Id(), z.Domain()))
			} else {
				modified = true
				logger.Infof("adding provider %q for hosted zone %q (%s)", name, z.Id(), z.Domain())
				this.addProviderForZone(z.Id(), name)
			}
			result[z.Id()] = zone
		}
	}

	if last != nil {
		name = last.ObjectName()
		old := this.providerzones[name]
		if old != nil {
			for n, z := range old {
				if result[n] == nil {
					modified = true
					this.removeProviderForZone(n, name)
					logger.Infof("removing provider %q for hosted zone %q (%s)", name, z.Id(), z.Domain())
					if !this.hasProvidersForZone(n) {
						logger.Infof("removing hosted zone %q (%s)", z.Id(), z.Domain())
						metrics.DeleteZone(z.Id())
						delete(this.zones, n)
					}
				}
			}
		}
		if modified {
			for _, m := range keeping {
				logger.Info(m)
			}
		}
	}
	this.providerzones[name] = result
	return modified
}

////////////////////////////////////////////////////////////////////////////////
// provider handling
////////////////////////////////////////////////////////////////////////////////

func (this *state) UpdateProvider(logger logger.LogContext, obj *dnsutils.DNSProviderObject) reconcile.Status {
	logger.Infof("reconcile PROVIDER")
	if !this.config.Factory.IsResponsibleFor(obj) {
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
		return status
	}

	entries := Entries{}
	if last == nil || !new.equivalentTo(last) {
		this.addEntriesForProvider(last, entries)
		this.addEntriesForProvider(new, entries)
	}
	this.registerSecret(logger, new.secret, new)

	if mod || (last != nil && !last.IsValid() && new.IsValid()) {
		logger.Infof("found %d zones: ", len(new.zones))
		for _, z := range new.zones {
			logger.Infof("    %s: %s", z.Id(), z.Domain())
			if len(z.ForwardedDomains()) > 0 {
				logger.Infof("        forwarded: %s", utils.Strings(z.ForwardedDomains()...))
			}
		}
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

func (this *state) TriggerEntriesByOwner(logger logger.LogContext, owners utils.StringSet) {
	for _, e := range this.GetEntriesByOwner(owners) {
		this.TriggerEntry(logger, e)
	}
}

func (this *state) GetEntriesByOwner(owners utils.StringSet) Entries {
	if len(owners) == 0 {
		return nil
	}
	this.lock.RLock()
	defer this.lock.RUnlock()

	entries := Entries{}
	for _, e := range this.entries {
		if owners.Contains(e.OwnerId()) {
			entries[e.ObjectName()] = e
		}
	}
	return entries
}

func (this *state) TriggerEntries(logger logger.LogContext, entries Entries) {
	for _, e := range this.entries {
		this.TriggerEntry(logger, e)
	}
}

func (this *state) TriggerEntry(logger logger.LogContext, e *Entry) {
	logger.Infof("trigger entry %s", e.ClusterKey())
	this.controller.EnqueueKey(e.ClusterKey())
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
		logger.Infof("deleting provider")
		entries := Entries{}
		zones := this.providerzones[obj.ObjectName()]
		for n, z := range zones {
			if this.isProviderForZone(n, pname) {
				this.addEntriesForZone(logger, entries, nil, z)
				providers := this.getProvidersForZone(n)
				if len(providers) == 1 {
					// if this is the last provider for this zone
					// it must be cleanuped before the provider is gone
					logger.Infof("provider is exclusively handling zone %q -> cleanup", n)
					err := this.reconcileZone(logger, n, Entries{}, nil, providers)
					if err != nil {
						logger.Errorf("cannot cleanup zone %q: %s", n, err)
					}
					metrics.DeleteZone(n)
					delete(this.zones, n)
				} else {
					// delete entries in hosted zone exclusively covered by this provider using
					// other provider for this zone
					this.triggerHostedZone(n)
				}
				this.removeProviderForZone(n, pname)
			}
		}
		this.TriggerEntries(logger, entries)
		err := this.registerSecret(logger, nil, cur)
		if err != nil {
			return reconcile.Delay(logger, err)
		}
		this.accountCache.Release(logger, cur.account, cur.ObjectName())
		delete(this.deleting, obj.ObjectName())
		delete(this.providerzones, obj.ObjectName())
		return reconcile.DelayOnError(logger, this.RemoveFinalizer(cur.Object()))
	}
	return reconcile.Succeeded(logger)
}

////////////////////////////////////////////////////////////////////////////////
// secret handling
////////////////////////////////////////////////////////////////////////////////

func (this *state) UpdateSecret(logger logger.LogContext, obj resources.Object) reconcile.Status {
	providers := this.GetSecretUsage(obj.ObjectName())
	if providers == nil || len(providers) == 0 {
		return reconcile.DelayOnError(logger, this.RemoveFinalizer(obj))
	}
	logger.Infof("reconcile SECRET")
	for _, p := range providers {
		logger.Infof("requeueing provider %q using secret %q", p.ObjectName(), obj.ObjectName())
		if err := this.controller.Enqueue(p); err != nil {
			panic(fmt.Sprintf("cannot enqueue provider %q: %s", p.Description(), err))
		}
	}
	return reconcile.Succeeded(logger)
}

////////////////////////////////////////////////////////////////////////////////
// entry handling
////////////////////////////////////////////////////////////////////////////////

func (this *state) UpdateEntry(logger logger.LogContext, object *dnsutils.DNSEntryObject) reconcile.Status {
	return this.updateEntry(logger, "reconcile", object)
}

func (this *state) DeleteEntry(logger logger.LogContext, object *dnsutils.DNSEntryObject) reconcile.Status {
	return this.updateEntry(logger, "delete", object)
}

func (this *state) updateEntry(logger logger.LogContext, op string, object *dnsutils.DNSEntryObject) reconcile.Status {
	logger.Debugf("%s ENTRY", op)
	old, new, err := this.AddEntry(logger, object)

	newzone, _ := this.GetZoneForName(new.DNSName())
	oldzone := ""
	if old != nil {
		oldzone, _ = this.GetZoneForName(old.DNSName())
	}

	if old != nil {
		if oldzone != "" && (err != nil || oldzone != newzone) {
			logger.Infof("dns name changed -> trigger old zone %q", oldzone)
			if this.HasFinalizer(old.object) {
				this.outdated[old.ObjectName()] = old
			}
			this.triggerHostedZone(oldzone)
		} else {
			logger.Infof("dns name changed to %q", new.DNSName())
		}
	}

	if err == nil {
		var provider DNSProvider
		provider, err = this.LookupProvider(new)
		if provider == nil && err == nil {
			if newzone != "" {
				err = fmt.Errorf("no matching %s provider found", this.GetHandlerFactory().TypeCode())
			}
		}
	}

	status := new.Update(logger, this, op, object, this.GetHandlerFactory().TypeCode(), newzone, err, this.config.TTL)

	if status.IsSucceeded() && new.IsValid() {
		if new.Interval() > 0 {
			status = status.RescheduleAfter(time.Duration(new.Interval()) * time.Second)
		}
		if object.IsDeleting() || (new.IsModified() && newzone != "") {
			logger.Infof("trigger zone %q", newzone)
			this.triggerHostedZone(newzone)
		}
	}
	return status
}

func (this *state) EntryDeleted(logger logger.LogContext, key resources.ObjectKey) reconcile.Status {
	this.lock.Lock()
	defer this.lock.Unlock()

	old := this.entries[key.ObjectName()]
	if old != nil {
		zoneid, _ := this.getZoneForName(old.DNSName())
		if zoneid != "" {
			logger.Infof("removing entry %q (%s[%s])", key.ObjectName(), old.DNSName(), zoneid)
			this.triggerHostedZone(zoneid)
		} else {
			logger.Infof("removing foreign entry %q (%s)", key.ObjectName(), old.DNSName())
		}
		this.cleanupEntry(logger, old)
	} else {
		logger.Infof("removing unknown entry %q", key.ObjectName())
	}
	return reconcile.Succeeded(logger)
}

func (this *state) cleanupEntry(logger logger.LogContext, e *Entry) {
	logger.Infof("cleanup old entry (duplicate=%t)", e.duplicate)
	this.entries.Delete(e)
	if !e.duplicate {
		var found *Entry
		for _, a := range this.entries {
			logger.Debugf("  checking %s(%s): dup:%t", a.ObjectName(), a.DNSName(), a.duplicate)
			if a.duplicate && a.DNSName() == e.DNSName() {
				if found == nil {
					found = a
				} else {
					if a.Before(found) {
						found = a
					}
				}
			}
		}
		if found == nil {
			logger.Infof("no duplicate found to reactivate")
			delete(this.dnsnames, e.DNSName())
		} else {
			old := this.dnsnames[found.DNSName()]
			if old != nil {
				logger.Infof("reactivate duplicate for %s: %s replacing %s", found.DNSName(), found.ObjectName(), e.ObjectName())
			} else {
				logger.Infof("reactivate duplicate for %s: %s", found.DNSName(), found.ObjectName())
			}
			found.duplicate = false
			this.dnsnames[e.DNSName()] = found
			this.GetController().Enqueue(found.object)
		}
	}
}

func (this *state) AddEntry(logger logger.LogContext, object *dnsutils.DNSEntryObject) (*Entry, *Entry, error) {
	this.lock.Lock()
	defer this.lock.Unlock()

	old, new := this.entries.Add(object, this)
	if old != nil {
		// DNS name changed -> clean up old dns name
		this.cleanupEntry(logger, old)
	}

	dnsname := object.GetDNSName()
	cur := this.dnsnames[dnsname]
	if dnsname != "" {
		if cur != nil {
			if cur.ObjectName() != new.ObjectName() {
				if cur.Before(new) {
					new.duplicate = true
					return old, new, fmt.Errorf("DNS name %q already busy for %q", dnsname, cur.ObjectName())
				} else {
					cur.duplicate = true
					logger.Warnf("DNS name %q already busy for %q, but this one was earlier", dnsname, cur.ObjectName())
					logger.Infof("reschedule %q for error update", cur.ObjectName())
					this.triggerKey(cur.ClusterKey())
				}
			}
		}
		this.dnsnames[dnsname] = new
	}
	return old, new, nil
}

////////////////////////////////////////////////////////////////////////////////
// zone reconcilation
////////////////////////////////////////////////////////////////////////////////

func (this *state) GetZoneInfo(logger logger.LogContext, zoneid string) (*dnsHostedZone, DNSProviders, Entries, DNSNames) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	zone := this.zones[zoneid]
	if zone == nil {
		return nil, nil, nil, nil
	}
	entries, stale := this.addEntriesForZone(logger, nil, nil, zone)
	return zone, this.getProvidersForZone(zoneid), entries, stale
}

func (this *state) ReconcileZone(logger logger.LogContext, zoneid string) reconcile.Status {
	zone, providers, entries, stale := this.GetZoneInfo(logger, zoneid)
	if zone == nil {
		return reconcile.Failed(logger, fmt.Errorf("zone %s not used anymore -> stop reconciling", zoneid))
	}
	if done, err := this.startZoneReconcilation(logger, zone, entries, stale, providers); done {
		return reconcile.DelayOnError(logger, err)
	}
	logger.Infof("reconciling zone %q (%s) already busy and skipped", zoneid, zone.Domain())
	return reconcile.Succeeded(logger).RescheduleAfter(10 * time.Second)
}

func (this *state) startZoneReconcilation(logger logger.LogContext, zone *dnsHostedZone, entries Entries, stale DNSNames, providers DNSProviders) (bool, error) {
	if zone.TestAndSetBusy() {
		logger.Infof("reconciling zone %q (%s) with %d entries", zone.Id(), zone.Domain(), len(entries))
		defer zone.Release()
		return true, this.reconcileZone(logger, zone.Id(), entries, stale, providers)
	}
	return false, nil
}

func (this *state) reconcileZone(logger logger.LogContext, zoneid string, entries Entries, stale DNSNames, providers DNSProviders) error {
	zone := this.zones[zoneid]
	if zone == nil {
		metrics.DeleteZone(zoneid)
		return nil
	}
	metrics.ReportZoneEntries(zone.ProviderType(), zoneid, len(entries))
	changes := NewChangeModel(logger, this.ownerCache.GetIds(), stale, this.config, zone, providers)
	err := changes.Setup()
	if err != nil {
		return err
	}
	modified := false
	for _, e := range entries {
		// TODO: err handling
		mod := false
		if e.IsDeleting() {
			mod, _ = changes.Delete(e.DNSName(), NewStatusUpdate(logger, e, this.GetController(), zoneid))
		} else {
			mod, _ = changes.Apply(e.DNSName(), NewStatusUpdate(logger, e, this.GetController(), zoneid), e.Targets()...)
		}
		modified = modified || mod
	}
	modified = changes.Cleanup(logger) || modified
	if modified {
		err = changes.Update(logger)
	}

	for k, e := range this.outdated {
		if e.activezone == zoneid {
			logger.Infof("cleanup outdated entry %q", k)
			if this.RemoveFinalizer(e.object) == nil {
				delete(this.outdated, k)
			}
		}
	}
	return err
}

////////////////////////////////////////////////////////////////////////////////
// OwnerIds
////////////////////////////////////////////////////////////////////////////////

func (this *state) UpdateOwner(logger logger.LogContext, owner *dnsutils.DNSOwnerObject) reconcile.Status {
	changed, active := this.ownerCache.UpdateOwner(owner)
	logger.Infof("update: changed owner ids %s, active owner ids %s", changed, active)
	this.TriggerEntriesByOwner(logger, changed)
	return reconcile.Succeeded(logger)
}

func (this *state) OwnerDeleted(logger logger.LogContext, key resources.ObjectKey) reconcile.Status {
	changed, active := this.ownerCache.DeleteOwner(key)
	logger.Infof("delete: changed owner ids %s, active owner ids %s", changed, active)
	this.TriggerEntriesByOwner(logger, changed)
	return reconcile.Succeeded(logger)
}
