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
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"strings"
	"sync"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/access"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
)

type state struct {
	lock       sync.Mutex
	controller controller.Interface
	config     Config

	pending utils.StringSet

	foreign         map[resources.ObjectName]*foreignProvider
	providers       map[resources.ObjectName]*dnsProviderVersion
	deleting        map[resources.ObjectName]*dnsProviderVersion
	secrets         map[resources.ObjectName]resources.ObjectNameSet
	zones           map[string]*dnsHostedZone
	zoneproviders   map[string]resources.ObjectNameSet
	providerzones   map[resources.ObjectName]map[string]*dnsHostedZone
	providersecrets map[resources.ObjectName]resources.ObjectName

	entries  Entries
	dnsnames map[string]*Entry

	initialized bool
}

var _ DNSState = &state{}

func NewDNSState(controller controller.Interface, config Config) DNSState {
	controller.Infof("using default ttl: %d", config.TTL)
	controller.Infof("using identifier : %s", config.Ident)
	controller.Infof("dry run mode     : %t", config.Dryrun)
	return &state{
		controller:      controller,
		config:          config,
		pending:         utils.StringSet{},
		foreign:         map[resources.ObjectName]*foreignProvider{},
		providers:       map[resources.ObjectName]*dnsProviderVersion{},
		deleting:        map[resources.ObjectName]*dnsProviderVersion{},
		zones:           map[string]*dnsHostedZone{},
		secrets:         map[resources.ObjectName]resources.ObjectNameSet{},
		zoneproviders:   map[string]resources.ObjectNameSet{},
		providerzones:   map[resources.ObjectName]map[string]*dnsHostedZone{},
		providersecrets: map[resources.ObjectName]resources.ObjectName{},
		entries:         Entries{},
		dnsnames:        map[string]*Entry{},
	}
}

func (this *state) Setup() {
	resources := this.controller.GetMainCluster().Resources()
	res, _ := resources.GetByExample(&api.DNSProvider{})
	{
		this.controller.Infof("setup providergroups")
		list, _ := res.ListCached(labels.Everything())
		for _, e := range list {
			p := dnsutils.DNSProvider(e)
			if this.GetHandlerFactory().IsResponsibleFor(p) {
				this.UpdateProvider(this.controller.NewContext("provider", p.ObjectName().String()), p)
			}
		}
	}
	{
		this.controller.Infof("setup entries")
		res, _ := resources.GetByExample(&api.DNSEntry{})
		list, _ := res.ListCached(labels.Everything())
		for _, e := range list {
			p := dnsutils.DNSEntry(e)
			this.UpdateEntry(this.controller.NewContext("entry", p.ObjectName().String()), p)
		}
	}
	this.initialized = true
	this.controller.Infof("setup done - starting reconcilation")
}

func (this *state) Start() {
	for c := range this.pending {
		this.controller.Infof("trigger %s", c)
		this.controller.EnqueueCommand(c)
	}
}

func (this *state) GetController() controller.Interface {
	return this.controller
}

func (this *state) GetConfig() Config {
	return this.config
}

func (this *state) GetHandlerFactory() DNSHandlerFactory {
	return this.config.Factory
}

func (this *state) GetProvidersForZone(zoneid string) DNSProviders {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.getProvidersForZone(zoneid)
}

func (this *state) HasProvidersForZone(zoneid string) bool {
	this.lock.Lock()
	defer this.lock.Unlock()
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

func (this *state) LookupProvider(dnsname string) DNSProvider {
	this.lock.Lock()
	defer this.lock.Unlock()

	var found DNSProvider
	match := -1
	for _, p := range this.providers {
		n := p.Match(dnsname)
		if n > 0 {
			if match < n {
				found = p
			}
		}
	}
	return found
}

func (this *state) GetSecretUsage(name resources.ObjectName) []resources.Object {
	this.lock.Lock()
	defer this.lock.Unlock()

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
			if len(oldp) == 0 {
				s, err := provider.Object().Resources().GetObjectInto(old, &corev1.Secret{})
				if err != nil {
					if !errors.IsNotFound(err) {
						logger.Warnf("cannot release secret %q for provider %q: %s", old, pname, err)
						return err
					}
				} else {
					logger.Infof("remove finalizer for unused secret %q", old)
					err := this.controller.RemoveFinalizer(s)
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

		s, err := provider.Object().Resources().GetObjectInto(secret, &corev1.Secret{})
		if err == nil {
			err = this.controller.SetFinalizer(s)
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
	this.lock.Lock()
	defer this.lock.Unlock()

	return this.providers[name]
}

func (this *state) GetZonesForProvider(name resources.ObjectName) dnsHostedZones {
	this.lock.Lock()
	defer this.lock.Unlock()
	return copyZones(this.providerzones[name])
}

func (this *state) GetEntriesForZone(zoneid string) Entries {
	this.lock.Lock()
	defer this.lock.Unlock()
	entries := Entries{}
	zone := this.zones[zoneid]
	if zone != nil {
		this.addEntriesForDomain(entries, zone.Domain())
	}
	return entries
}

func (this *state) addEntriesForDomain(entries Entries, domain string) Entries {
	for dns, e := range this.dnsnames {
		if e.IsValid() {
			if dnsutils.Match(dns, domain) {
				entries[e.ObjectName()] = e
			}
		}
	}
	return entries
}

func (this *state) GetZoneForEntry(e *Entry) string {
	if !e.IsValid() {
		return ""
	}
	zoneid, _ := this.GetZoneForName(e.DNSName())
	return zoneid
}

func (this *state) GetZoneForName(name string) (string, int) {
	this.lock.Lock()
	defer this.lock.Unlock()

	return this.getZoneForName(name)
}

func (this *state) getZoneForName(hostname string) (string, int) {
	length := 0
	found := ""
	for zoneid, zone := range this.zones {
		name := zone.Domain()
		if dnsutils.Match(hostname, name) {
			if length < len(name) {
				length = len(name)
				found = zoneid
			}
		}
	}
	return found, length
}

func (this *state) triggerHostedZone(name string) {
	cmd := "hostedzone:" + name
	if this.controller.IsReady() {
		this.controller.EnqueueCommand(cmd)
	} else {
		this.pending.Add(cmd)
	}
}

func (this *state) DecodeZoneCommand(name string) string {
	if strings.HasPrefix(name, "hostedzone:") {
		return name[len("hostedzone:"):]
	}
	return ""
}

func (this *state) updateZones(logger logger.LogContext, provider *dnsProviderVersion) bool {
	keeping := []string{}
	modified := false
	result := map[string]*dnsHostedZone{}
	for _, z := range provider.zoneinfos {
		zone := this.zones[z.Id]
		if zone == nil {
			modified = true
			zone = newDNSHostedZone(z.Id, z.Domain)
			this.zones[z.Id] = zone
			logger.Infof("adding hosted zone %q (%s)", z.Id, z.Domain)
			this.triggerHostedZone(zone.Id())
		}
		zone.update(z)

		if this.isProviderForZone(z.Id, provider.ObjectName()) {
			keeping = append(keeping, fmt.Sprintf("keeping provider %q for hosted zone %q (%s)", provider.ObjectName(), z.Id, z.Domain))
		} else {
			modified = true
			logger.Infof("adding provider %q for hosted zone %q (%s)", provider.ObjectName(), z.Id, z.Domain)
			this.addProviderForZone(z.Id, provider.ObjectName())
		}
		result[z.Id] = zone
	}

	old := this.providerzones[provider.ObjectName()]
	if old != nil {
		for n, z := range old {
			if result[n] == nil {
				modified = true
				this.removeProviderForZone(n, provider.ObjectName())
				logger.Infof("removing provider %q for hosted zone %q (%s)", provider.ObjectName(), z.Id, z.Domain)
				if !this.hasProvidersForZone(n) {
					logger.Infof("removing hosted zone %q (%s)", z.Id, z.Domain)
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
	this.providerzones[provider.ObjectName()] = result
	return modified
}

////////////////////////////////////////////////////////////////////////////////
// provider handling

func (this *state) UpdateProvider(logger logger.LogContext, obj *dnsutils.DNSProviderObject) reconcile.Status {
	logger.Infof("reconcile PROVIDER")
	if !this.config.Factory.IsResponsibleFor(obj) {
		return this._UpdateForeignProvider(logger, obj)
	}
	return this._UpdateLocalProvider(logger, obj)
}

func (this *state) _UpdateLocalProvider(logger logger.LogContext, obj *dnsutils.DNSProviderObject) reconcile.Status {
	err := this.controller.SetFinalizer(obj)
	if err != nil {
		return reconcile.Delay(logger, fmt.Errorf("cannot set finalizer: %s", err))
	}

	p := this.GetProvider(obj.ObjectName())

	var last *dnsProviderVersion
	if p != nil {
		last = p.(*dnsProviderVersion)
	}

	new, status := updateDNSProvider(logger, this, obj, last)

	if new==nil {
		return status
	}
	entries := Entries{}

	this.lock.Lock()
	defer this.lock.Unlock()

	if last == nil || !new.equivalentTo(last) {
		this.addEntriesForProvider(last, entries)
		this.addEntriesForProvider(new, entries)
		this.providers[new.ObjectName()] = new
	}
	this.registerSecret(logger, new.secret, new)

	mod := this.updateZones(logger, new)
	if mod {
		logger.Infof("found %d zones: ", len(new.zoneinfos))
		for _, z := range new.zoneinfos {
			logger.Infof("    %s: %s", z.Id, z.Domain)
		}
	}
	this.triggerEntries(logger, entries)
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

func (this *state) triggerEntries(logger logger.LogContext, entries Entries) {
	for _, e := range this.entries {
		logger.Infof("trigger entry %s", e.ClusterKey())
		this.controller.EnqueueKey(e.ClusterKey())
	}
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
		if cur.handler == nil {
			panic(fmt.Sprintf("OOPS, no handler for %s", pname))
		}
		entries := Entries{}
		zones := this.providerzones[obj.ObjectName()]
		for n, z := range zones {
			if this.isProviderForZone(n, pname) {
				this.addEntriesForDomain(entries, z.Domain())
				providers := this.getProvidersForZone(n)
				if len(providers) == 1 {
					// if this is the last provider for this zone
					// it must be cleanuped before the provider is gone
					logger.Infof("provider is exclusively handling zone %q -> cleanup", n)
					err := this.reconcileZone(logger, n, Entries{}, providers)
					if err != nil {
						logger.Errorf("cannot cleanup zone %q: %s", n, err)
					}
					delete(this.zones, n)
				} else {
					// delete entries in hosted zone exclusively covered by this provider using
					// other provider for this zone
					this.triggerHostedZone(n)
				}
				this.removeProviderForZone(n, pname)
			}
		}
		for _, e := range entries {
			this.controller.Enqueue(e.object)
		}
		err := this.registerSecret(logger, nil, cur)
		if err != nil {
			return reconcile.Delay(logger, err)
		}
		delete(this.deleting, obj.ObjectName())
		delete(this.providerzones, obj.ObjectName())
		return reconcile.DelayOnError(logger, this.controller.RemoveFinalizer(cur.Object()))
	}
	return reconcile.Succeeded(logger)
}

////////////////////////////////////////////////////////////////////////////////
// secret handling

func (this *state) UpdateSecret(logger logger.LogContext, obj resources.Object) reconcile.Status {
	providers := this.GetSecretUsage(obj.ObjectName())
	if providers == nil || len(providers) == 0 {
		return reconcile.DelayOnError(logger, this.controller.RemoveFinalizer(obj))
	}
	logger.Infof("reconcile SECRET")
	for _, p := range providers {
		logger.Infof("requeueing provider %q using secret %q", p.ObjectName(), obj.ObjectName())
		if err := this.controller.Enqueue(p); err != nil {
			panic(fmt.Sprintf("cannot enqueue provider %q: %s", p.Description(), err))
		}
	}
	return reconcile.Succeeded(logger).Stop()
}

////////////////////////////////////////////////////////////////////////////////
// entry handling

func (this *state) UpdateEntry(logger logger.LogContext, object *dnsutils.DNSEntryObject) reconcile.Status {
	logger.Infof("reconcile ENTRY")
	old, new, err := this.AddEntry(logger, object)

	newzone, _ := this.GetZoneForName(new.DNSName())
	if old != nil {
		oldzone, _ := this.GetZoneForName(old.DNSName())
		if oldzone != "" && (err != nil || oldzone != newzone) {
			logger.Infof("dns name changed -> trigger old zone %q", oldzone)
			this.triggerHostedZone(oldzone)
		} else {
			logger.Infof("dns name changed to %q", new.DNSName())
		}
	}

	if err == nil {
		provider := this.LookupProvider(object.GetDNSName())
		if provider != nil {
			owners := object.GetOwners()
			if len(owners) > 0 {
				for o := range owners {
					ok, msg, aerr := access.Allowed(o, "use", provider.Object().ClusterKey())
					if !ok {
						if aerr != nil {
							err = fmt.Errorf("%s: %s: %s", o, msg, err)
						} else {
							err = fmt.Errorf("%s: %s", o, msg)
						}
					}
				}
			} else {
				o := object.ClusterKey()
				ok, msg, aerr := access.Allowed(o, "use", provider.Object().ClusterKey())
				if !ok {
					if aerr != nil {
						err = fmt.Errorf("%s: %s: %s", o, msg, err)
					} else {
						err = fmt.Errorf("%s: %s", o, msg)
					}
				}
			}
		} else {
			if newzone!="" {
				err = fmt.Errorf("no matching %s provider found", this.GetHandlerFactory().TypeCode())
			}
		}
	}
	status := new.Update(logger, object, this.GetHandlerFactory().TypeCode(), newzone, err)

	if status.IsSucceeded() && new.IsValid() {
		if new.Interval() > 0 {
			this.controller.Enqueue(object.Object)
		}
		if new.IsModified() && newzone != "" {
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

	old, new := this.entries.Add(object)
	if old != nil {
		// DNS name changed -> cleam up old dns name
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
					this.controller.Enqueue(cur.object)
				}
			}
		}
		this.dnsnames[dnsname] = new
	}
	return old, new, nil
}

////////////////////////////////////////////////////////////////////////////////
// zone reconcilation

func (this *state) GetZoneInfo(zoneid string) (*dnsHostedZone, DNSProviders, Entries) {
	this.lock.Lock()
	defer this.lock.Unlock()

	zone := this.zones[zoneid]
	if zone == nil {
		return nil, nil, nil
	}
	return zone, this.getProvidersForZone(zoneid), this.addEntriesForDomain(Entries{}, zone.Domain())
}

func (this *state) ReconcileZone(logger logger.LogContext, zoneid string) reconcile.Status {
	zone, providers, entries := this.GetZoneInfo(zoneid)
	if zone == nil {
		return reconcile.Failed(logger, fmt.Errorf("zone %s not used anymore -> stop reconciling", zoneid))
	}
	if zone.TestAndSetBusy() {
		logger.Infof("reconciling zone %q (%s) with %d entries entries", zoneid, zone.Domain(), len(entries))
		defer zone.Release()
		return reconcile.DelayOnError(logger, this.reconcileZone(logger, zoneid, entries, providers))
	}
	logger.Infof("reconciling zone %q (%s) already busy and skipped", zoneid, zone.Domain())
	return reconcile.Succeeded(logger)
}

func (this *state) reconcileZone(logger logger.LogContext, zoneid string, entries Entries, providers DNSProviders) error {
	changes := NewChangeModel(logger, this.config, zoneid, providers)
	err := changes.Setup()
	if err != nil {
		return err
	}
	modified := false
	for _, e := range entries {
		// TODO: err handling
		mod, _ := changes.Apply(e.DNSName(), NewStatusUpdate(logger, e), e.Targets()...)
		modified = modified || mod
	}
	modified = modified || changes.Cleanup(logger)
	if modified {
		err = changes.Update(logger)
	}
	return err
}
