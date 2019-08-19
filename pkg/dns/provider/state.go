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
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/resources/access"
	"strings"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/server/metrics"
	"k8s.io/apimachinery/pkg/runtime"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
)

type DNSNames map[string]*Entry

type zoneReconcilation struct {
	zone      *dnsHostedZone
	providers DNSProviders
	entries   Entries
	stale     DNSNames
	dedicated bool
	deleting  bool
}

type state struct {
	lock sync.RWMutex

	context Context

	classes *dnsutils.Classes
	config  Config

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

func NewDNSState(ctx Context, classes *dnsutils.Classes, config Config) *state {
	ctx.Infof("responsible for classes:     %s (%s)", classes, classes.Main())
	ctx.Infof("availabled providers types   %s", config.Factory.TypeCodes())
	ctx.Infof("enabled providers types:     %s", config.Enabled)
	ctx.Infof("using default ttl:           %d", config.TTL)
	ctx.Infof("using identifier:            %s", config.Ident)
	ctx.Infof("dry run mode:                %t", config.Dryrun)
	ctx.Infof("reschedule delay:            %v", config.RescheduleDelay)
	ctx.Infof("zone cache ttl for zones:    %v", config.CacheTTL)
	ctx.Infof("zone cache persist dir:      %s", config.CacheDir)
	ctx.Infof("disable zone state caching:  %t", !config.ZoneStateCaching)
	return &state{
		classes:         classes,
		context:         ctx,
		config:          config,
		accountCache:    NewAccountCache(config.CacheTTL, config.CacheDir),
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
	processors, err := this.context.GetIntOption(OPT_SETUP)
	if err != nil || processors <= 0 {
		processors = 5
	}
	this.context.Infof("using %d parallel workers for initialization", processors)
	this.setupFor(&api.DNSProvider{}, "providers", func(e resources.Object) {
		p := dnsutils.DNSProvider(e)
		if this.GetHandlerFactory().IsResponsibleFor(p) {
			this.UpdateProvider(this.context.NewContext("provider", p.ObjectName().String()), p)
		}
	}, processors)
	this.setupFor(&api.DNSOwner{}, "owners", func(e resources.Object) {
		p := dnsutils.DNSOwner(e)
		this.UpdateOwner(this.context.NewContext("owner", p.ObjectName().String()), p)
	}, processors)
	this.setupFor(&api.DNSEntry{}, "entries", func(e resources.Object) {
		p := dnsutils.DNSEntry(e)
		this.UpdateEntry(this.context.NewContext("entry", p.ObjectName().String()), p)
	}, processors)

	this.initialized = true
	this.context.Infof("setup done - starting reconcilation")
}

func (this *state) setupFor(obj runtime.Object, msg string, exec func(resources.Object), processors int) {
	this.context.Infof("### setup %s", msg)
	res, _ := this.context.GetByExample(obj)
	list, _ := res.ListCached(labels.Everything())
	dnsutils.ProcessElements(list, func(e resources.Object) {
		if this.IsResponsibleFor(this.context, e) {
			exec(e)
		}
	}, processors)
}

func (this *state) Start() {
	for c := range this.pending {
		this.context.Infof("trigger %s", c)
		this.context.EnqueueCommand(c)
	}

	for key := range this.pendingKeys {
		this.context.Infof("trigger key %s/%s", key.Namespace(), key.Name())
		this.context.EnqueueKey(key)
		delete(this.pendingKeys, key)
	}
}

func (this *state) HasFinalizer(obj resources.Object) bool {
	return this.context.HasFinalizer(obj)
}

func (this *state) SetFinalizer(obj resources.Object) error {
	return this.context.SetFinalizer(obj)
}

func (this *state) RemoveFinalizer(obj resources.Object) error {
	return this.context.RemoveFinalizer(obj)
}

func (this *state) GetContext() Context {
	return this.context
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

func (this *state) hasProviders() bool {
	return len(this.providers) > 0
}

func (this *state) LookupProvider(e *EntryVersion) (DNSProvider, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.lookupProvider(e.Object())
}

func (this *state) lookupProvider(e *dnsutils.DNSEntryObject) (DNSProvider, error) {
	var err error
	var found DNSProvider
	match := -1
	for _, p := range this.providers {
		if p.IsValid() {
			n := p.Match(e.GetDNSName())
			if n > 0 {
				if match < n {
					err = access.CheckAccess(e, p.Object())
					if err == nil {
						found = p
						match = n
					}
				}
			}
		}
	}
	if found != nil {
		err = nil
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
		if o != nil {
			result = append(result, o.object)
		}
	}
	return result
}

func (this *state) registerSecret(logger logger.LogContext, secret resources.ObjectName, provider *dnsProviderVersion) (bool, error) {
	pname := provider.ObjectName()
	old := this.providersecrets[pname]

	if old != nil && old != secret {
		oldp := this.secrets[old]
		if oldp.Contains(pname) {
			logger.Infof("releasing secret %q for provider %q", old, pname)
			if len(oldp) <= 1 {
				r, err := provider.Object().Resources().Get(&corev1.Secret{})
				if err != nil {
					logger.Warnf("cannot release secret %q for provider %q: %s", old, pname, err)
					return true, err
				}
				s, err := r.GetCached(old)
				if err != nil {
					if !errors.IsNotFound(err) {
						logger.Warnf("cannot release secret %q for provider %q: %s", old, pname, err)
						return true, err
					}
				} else {
					logger.Infof("remove finalizer for unused secret %q", old)
					err := this.RemoveFinalizer(s)
					if err != nil && !errors.IsNotFound(err) {
						logger.Warnf("cannot release secret %q for provider %q: %s", old, pname, err)
						return true, err
					}
				}
				delete(this.secrets, old)
			} else {
				delete(oldp, pname)
			}
		}
	}
	mod := false
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
			mod = true
		}

		r, err := provider.Object().Resources().Get(&corev1.Secret{})
		s, err := r.GetCached(secret)
		if err == nil {
			err = this.SetFinalizer(s)
		}
		if err != nil {
			if errors.IsNotFound(err) {
				return mod, fmt.Errorf("secret %q for provider %q not found", secret, pname)
			} else {
				return mod, fmt.Errorf("cannot set finalizer for secret %q for provider %q: %s", secret, pname, err)
			}
		}
	} else {
		delete(this.providersecrets, pname)
	}
	return mod, nil
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

func (this *state) GetEntriesForZone(logger logger.LogContext, zoneid string) (Entries, DNSNames, bool) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	entries := Entries{}
	zone := this.zones[zoneid]
	if zone != nil {
		return this.addEntriesForZone(logger, entries, DNSNames{}, zone)
	}
	return entries, nil, false
}

func (this *state) addEntriesForZone(logger logger.LogContext, entries Entries, stale DNSNames, zone *dnsHostedZone) (Entries, DNSNames, bool) {
	if entries == nil {
		entries = Entries{}
	}
	if stale == nil {
		stale = DNSNames{}
	}
	deleting := true
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
			provider, err := this.lookupProvider(e.Object())
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
					deleting = deleting || e.IsDeleting()
					entries[e.ObjectName()] = e
				} else {
					logger.Infof("entry %q(%s) is inactive", e.ObjectName(), e.DNSName())
				}
			}
		} else {
			if !e.IsDeleting() {
				if utils.StringValue(e.object.Status().Provider) != "" {
					logger.Infof("invalid entry %q (%s): %s (%s)", e.ObjectName(), e.DNSName(), e.State(), e.Message())
				}
				stale[e.DNSName()] = e
			}
		}
	}
	return entries, stale, deleting
}

func (this *state) GetZoneForEntry(e *Entry) string {
	if !e.IsValid() {
		return ""
	}
	zoneid, _, _ := this.GetZoneForName(e.DNSName())
	return zoneid
}

func (this *state) GetZoneForName(name string) (string, string, int) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	return this.getZoneForName(name)
}

func (this *state) getZoneForName(hostname string) (string, string, int) {
	var found *dnsHostedZone
	length := 0
loop:
	for _, zone := range this.zones {
		name := zone.Domain()
		if dnsutils.Match(hostname, name) {
			for _, f := range zone.ForwardedDomains() {
				if dnsutils.Match(hostname, f) {
					continue loop
				}
			}
			if length < len(name) {
				length = len(name)
				found = zone
			}
		}
	}
	if found != nil {
		return found.Id(), found.ProviderType(), length
	}
	return "", "", length
}

func (this *state) triggerHostedZone(name string) {
	cmd := HOSTEDZONE_PREFIX + name
	if this.context.IsReady() {
		this.context.EnqueueCommand(cmd)
	} else {
		this.pending.Add(cmd)
	}
}

func (this *state) triggerKey(key resources.ClusterObjectKey) {
	if this.context.IsReady() {
		this.context.EnqueueKey(key)
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

func (this *state) RefineLogger(logger logger.LogContext, ptype string) logger.LogContext {
	if len(this.config.Enabled) > 1 && ptype != "" {
		logger = logger.NewContext("provider", ptype)
	}
	return logger
}

////////////////////////////////////////////////////////////////////////////////
// provider handling
////////////////////////////////////////////////////////////////////////////////

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
	if logger != nil {
		logger.Infof("trigger entry %s", e.ClusterKey())
	}
	this.context.EnqueueKey(e.ClusterKey())
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
		logger.Infof("deleting PROVIDER")
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

					done, err := this.StartZoneReconcilation(logger, &zoneReconcilation{
						zone:z,
						providers: providers,
						entries: Entries{},
						stale: nil,
						dedicated: false,
						deleting: false,
					})
					if !done {
						return reconcile.Delay(logger, fmt.Errorf("zone reconcilation busy -> delay deletion"))
					}
					if err != nil {
						logger.Errorf("zone cleanup failed: %s", err)
						return reconcile.Delay(logger, fmt.Errorf("zone reconcilation failed -> delay deletion"))
					}
					metrics.DeleteZone(n)
					delete(this.zones, n)
				} else {
					// delete entries in hosted zone exclusively covered by this provider using
					// other provider for this zone
					logger.Infof("delegate zone cleanup to other provider")
					this.triggerHostedZone(n)
				}
				this.removeProviderForZone(n, pname)
			}
		}
		this.TriggerEntries(logger, entries)
		_, err := this.registerSecret(logger, nil, cur)
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
		if err := this.context.Enqueue(p); err != nil {
			panic(fmt.Sprintf("cannot enqueue provider %q: %s", p.Description(), err))
		}
	}
	return reconcile.Succeeded(logger)
}

////////////////////////////////////////////////////////////////////////////////
// entry handling
////////////////////////////////////////////////////////////////////////////////

func (this *state) UpdateEntry(logger logger.LogContext, object *dnsutils.DNSEntryObject) reconcile.Status {
	return this.HandleUpdateEntry(logger, "reconcile", object)
}

func (this *state) DeleteEntry(logger logger.LogContext, object *dnsutils.DNSEntryObject) reconcile.Status {
	return this.HandleUpdateEntry(logger, "delete", object)
}

func (this *state) GetEntry(name resources.ObjectName) *Entry {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.entries[name]
}

func (this *state) smartInfof(logger logger.LogContext, format string, args ...interface{}) {
	if this.hasProviders() {
		logger.Infof(format, args...)
	} else {
		logger.Debugf(format, args...)
	}
}

func (this *state) AddEntryVersion(logger logger.LogContext, v *EntryVersion, status reconcile.Status) (*Entry, reconcile.Status) {
	this.lock.Lock()
	defer this.lock.Unlock()

	var new *Entry
	old := this.entries[v.ObjectName()]
	if old == nil {
		new = NewEntry(v, this)
	} else {
		new = old.Update(logger, v)
	}

	if v.IsDeleting() {
		var err error
		if old != nil {
			this.cleanupEntry(logger, old)
		}
		if new.valid {
			if this.zones[new.activezone] != nil {
				if this.HasFinalizer(new.Object()) {
					logger.Infof("deleting delayed until entry deleted in provider")
					this.outdated[new.ObjectName()] = new
					return new, reconcile.Succeeded(logger)
				}
			} else {
				logger.Infof("dns zone '%s' of deleted entry gone", old.ZoneId())
				err = this.RemoveFinalizer(v.object)
			}
		} else {
			this.smartInfof(logger, "deleting yet unmanaged or errorneous entry")
			err = this.RemoveFinalizer(v.object)
		}
		if err != nil {
			this.entries[v.ObjectName()] = new
		}
		return new, reconcile.DelayOnError(logger, err)
	}

	this.entries[v.ObjectName()] = new

	if old != nil && old != new {
		// DNS name changed -> clean up old dns name
		logger.Infof("dns name changed to %q", new.DNSName())
		this.cleanupEntry(logger, old)
		if old.activezone != "" && old.activezone != new.ZoneId() {
			if this.zones[old.activezone] != nil {
				logger.Infof("dns zone changed -> trigger old zone '%s'", old.ZoneId())
				this.triggerHostedZone(old.activezone)
			}
		}
	}

	if !this.IsManaging(v) {
		this.smartInfof(logger, "foreign zone %s(%s) -> skip reconcilation", utils.StringValue(v.status.Zone), utils.StringValue(v.status.ProviderType))
		return nil, status
	}

	dnsname := v.DNSName()
	cur := this.dnsnames[dnsname]
	if dnsname != "" {
		if cur != nil {
			if cur.ObjectName() != new.ObjectName() {
				if cur.Before(new) {
					new.duplicate = true
					new.modified = false
					err := fmt.Errorf("DNS name %q already busy for %q", dnsname, cur.ObjectName())
					logger.Warnf("%s", err)
					if status.IsSucceeded() {
						_, err := v.UpdateStatus(logger, api.STATE_ERROR, err.Error())
						if err != nil {
							return new, reconcile.DelayOnError(logger, err)
						}
					}
					return new, status
				} else {
					cur.duplicate = true
					cur.modified = false
					logger.Warnf("DNS name %q already busy for %q, but this one was earlier", dnsname, cur.ObjectName())
					logger.Infof("reschedule %q for error update", cur.ObjectName())
					this.triggerKey(cur.ClusterKey())
				}
			}
		}
		if new.valid && new.status.State != api.STATE_READY && new.status.State != api.STATE_PENDING {
			msg := fmt.Sprintf("activating for %s", new.DNSName())
			logger.Info(msg)
			new.UpdateStatus(logger, api.STATE_PENDING, msg)
		}
		this.dnsnames[dnsname] = new
	}

	return new, status
}

func (this *state) IsManaging(v *EntryVersion) bool {
	if v.status.ProviderType == nil {
		return false
	}
	return this.GetHandlerFactory().TypeCodes().Contains(*v.status.ProviderType)
}

func (this *state) EntryPremise(e *dnsutils.DNSEntryObject) (*EntryPremise, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	provider, err := this.lookupProvider(e)
	zoneid, ptype, _ := this.getZoneForName(e.GetDNSName())

	return &EntryPremise{
		this.config.Enabled,
		ptype,
		provider,
		zoneid,
	}, err
}

func (this *state) HandleUpdateEntry(logger logger.LogContext, op string, object *dnsutils.DNSEntryObject) reconcile.Status {
	old := this.GetEntry(object.ObjectName())
	if old != nil {
		old.lock.Lock()
		defer old.lock.Unlock()
	}

	p, err := this.EntryPremise(object)
	if p.provider == nil && err == nil {
		if p.zoneid != "" {
			err = fmt.Errorf("no matching provider for zone '%s' found", p.zoneid)
		}
	}

	logger = this.RefineLogger(logger, p.ptype)
	v := NewEntryVersion(object, old)
	status := v.Setup(logger, this, p, op, err, this.config, old)
	new, status := this.AddEntryVersion(logger, v, status)

	if new != nil {
		if status.IsSucceeded() && new.IsValid() {
			if new.Interval() > 0 {
				status = status.RescheduleAfter(time.Duration(new.Interval()) * time.Second)
			}
		}
		if new.IsModified() && new.ZoneId() != "" {
			this.smartInfof(logger, "trigger zone %q", new.ZoneId())
			this.triggerHostedZone(new.ZoneId())
		} else {
			logger.Debugf("skipping trigger zone %q because entry not modified", new.ZoneId())
		}
	}

	if !object.IsDeleting() {
		check, _ := this.EntryPremise(object)
		if !check.Equals(p) {
			logger.Infof("%s -> repeat reconcilation", p.NotifyChange(check))
			return reconcile.Repeat(logger)
		}
	}
	return status
}

func (this *state) EntryDeleted(logger logger.LogContext, key resources.ObjectKey) reconcile.Status {
	this.lock.Lock()
	defer this.lock.Unlock()

	old := this.entries[key.ObjectName()]
	if old != nil {
		zoneid, _, _ := this.getZoneForName(old.DNSName())
		if zoneid != "" {
			logger.Infof("removing entry %q (%s[%s])", key.ObjectName(), old.DNSName(), zoneid)
			this.triggerHostedZone(zoneid)
		} else {
			this.smartInfof(logger, "removing foreign entry %q (%s)", key.ObjectName(), old.DNSName())
		}
		this.cleanupEntry(logger, old)
	} else {
		logger.Debugf("removing unknown entry %q", key.ObjectName())
	}
	return reconcile.Succeeded(logger)
}

func (this *state) cleanupEntry(logger logger.LogContext, e *Entry) {
	this.smartInfof(logger, "cleanup old entry (duplicate=%t)", e.duplicate)
	this.entries.Delete(e)
	if this.dnsnames[e.DNSName()] == e {
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
		} else {
			old := this.dnsnames[found.DNSName()]
			msg := ""
			if old != nil {
				msg = fmt.Sprintf("reactivate duplicate for %s: %s replacing %s", found.DNSName(), found.ObjectName(), e.ObjectName())
			} else {
				msg = fmt.Sprintf("reactivate duplicate for %s: %s", found.DNSName(), found.ObjectName())
			}
			logger.Info(msg)
			found.Trigger(nil)
		}
		delete(this.dnsnames, e.DNSName())
	}
}

////////////////////////////////////////////////////////////////////////////////
// zone reconcilation
////////////////////////////////////////////////////////////////////////////////

func (this *state) GetZoneInfo(logger logger.LogContext, zoneid string) (*dnsHostedZone, DNSProviders, Entries, DNSNames, bool) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	zone := this.zones[zoneid]
	if zone == nil {
		return nil, nil, nil, nil, false
	}
	entries, stale, deleting := this.addEntriesForZone(logger, nil, nil, zone)
	return zone, this.getProvidersForZone(zoneid), entries, stale, deleting
}

func (this *state) GetZoneReconcilation(logger logger.LogContext, zoneid string) (time.Duration, bool, *zoneReconcilation) {
	req := &zoneReconcilation{}

	this.lock.RLock()
	defer this.lock.RUnlock()

	hasProviders := this.hasProviders()
	zone := this.zones[zoneid]
	if zone == nil {
		return 0, hasProviders, nil
	}
	now := time.Now()
	req.zone = zone
	if now.Before(zone.next) {
		return zone.next.Sub(now), hasProviders, req
	}
	req.entries, req.stale, req.deleting = this.addEntriesForZone(logger, nil, nil, zone)
	req.providers = this.getProvidersForZone(zoneid)
	return 0, hasProviders, req
}

func (this *state) ReconcileZone(logger logger.LogContext, zoneid string) reconcile.Status {
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
	if done, err := this.StartZoneReconcilation(logger, req); done {
		return reconcile.DelayOnError(logger, err)
	}
	logger.Infof("reconciling zone %q (%s) already busy and skipped", zoneid, req.zone.Domain())
	return reconcile.Succeeded(logger).RescheduleAfter(10 * time.Second)
}

func (this *state) StartZoneReconcilation(logger logger.LogContext, req *zoneReconcilation) (bool, error) {
	if req.deleting {
		ctxutil.Tick(this.GetContext().GetContext(),controller.DeletionActivity)
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
		list.Lock()
		defer func() {
			logger.Infof("unlocking %d entries", len(list))
			list.Unlock()
		}()
		return true, this.reconcileZone(logger, req)
	}
	return false, nil
}

func (this *state) reconcileZone(logger logger.LogContext, req *zoneReconcilation) error {
	zoneid := req.zone.Id()
	req.zone = this.zones[zoneid]
	if req.zone == nil {
		metrics.DeleteZone(zoneid)
		return nil
	}
	req.zone.next = time.Now().Add(this.config.Delay)
	metrics.ReportZoneEntries(req.zone.ProviderType(), zoneid, len(req.entries))
	logger.Infof("reconcile ZONE %s (%s) for %d dns entries (%d stale)", req.zone.Id(), req.zone.Domain(), len(req.entries), len(req.stale))
	changes := NewChangeModel(logger, this.ownerCache.GetIds(), req, this.config)
	err := changes.Setup()
	if err != nil {
		return err
	}
	modified := false
	for _, e := range req.entries {
		// TODO: err handling
		mod := false
		if e.IsDeleting() {
			mod, _ = changes.Delete(e.DNSName(), NewStatusUpdate(logger, e, this.GetContext()))
		} else {
			mod, _ = changes.Apply(e.DNSName(), NewStatusUpdate(logger, e, this.GetContext()), e.Targets()...)
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
			err := this.RemoveFinalizer(e.object)
			if err == nil || errors.IsNotFound(err) {
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
