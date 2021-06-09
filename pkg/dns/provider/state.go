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
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	"github.com/gardener/external-dns-management/pkg/server/metrics"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/access"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"k8s.io/apimachinery/pkg/labels"
)

type DNSNames map[string]*Entry

type zoneReconciliation struct {
	zone      *dnsHostedZone
	providers DNSProviders
	entries   Entries
	ownerIds  utils.StringSet
	stale     DNSNames
	dedicated bool
	deleting  bool
	fhandler  FinalizerHandler
}

type setup struct {
	lock        sync.Mutex
	pending     utils.StringSet
	pendingKeys resources.ClusterObjectKeySet
}

func newSetup() *setup {
	return &setup{
		pending:     utils.StringSet{},
		pendingKeys: resources.ClusterObjectKeySet{},
	}
}

func (this *setup) AddCommand(cmd ...string) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.pending.Add(cmd...)
}

func (this *setup) AddKey(key ...resources.ClusterObjectKey) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.pendingKeys.Add(key...)
}

func (this *setup) Start(context Context) {
	this.lock.Lock()
	defer this.lock.Unlock()
	for c := range this.pending {
		context.Infof("trigger %s", c)
		context.EnqueueCommand(c)
	}

	for key := range this.pendingKeys {
		context.Infof("trigger key %s/%s", key.Namespace(), key.Name())
		context.EnqueueKey(key)
	}
	this.pending = nil
	this.pendingKeys = nil
}

////////////////////////////////////////////////////////////////////////////////

type state struct {
	lock sync.RWMutex

	setup *setup

	context   Context
	ownerresc resources.Interface
	ownerupd  chan OwnerCounts

	classes *controller.Classes
	config  Config

	realms access.RealmTypes

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

	entries         Entries
	outdated        *synchronizedEntries
	blockingEntries map[resources.ObjectName]time.Time

	dnsnames   DNSNames
	references *References

	initialized bool
}

func NewDNSState(ctx Context, ownerresc resources.Interface, classes *controller.Classes, config Config) *state {
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

	realms := access.RealmTypes{"use": access.NewRealmType(dns.REALM_ANNOTATION)}

	return &state{
		setup:           newSetup(),
		classes:         classes,
		context:         ctx,
		ownerresc:       ownerresc,
		config:          config,
		realms:          realms,
		accountCache:    NewAccountCache(config.CacheTTL, config.CacheDir, config.Options),
		ownerCache:      NewOwnerCache(ctx, &config),
		foreign:         map[resources.ObjectName]*foreignProvider{},
		providers:       map[resources.ObjectName]*dnsProviderVersion{},
		deleting:        map[resources.ObjectName]*dnsProviderVersion{},
		zones:           map[string]*dnsHostedZone{},
		secrets:         map[resources.ObjectName]resources.ObjectNameSet{},
		zoneproviders:   map[string]resources.ObjectNameSet{},
		providerzones:   map[resources.ObjectName]map[string]*dnsHostedZone{},
		providersecrets: map[resources.ObjectName]resources.ObjectName{},
		entries:         Entries{},
		outdated:        newSynchronizedEntries(),
		blockingEntries: map[resources.ObjectName]time.Time{},
		dnsnames:        map[string]*Entry{},
		references:      NewReferenceCache(),
	}
}

func (this *state) IsResponsibleFor(logger logger.LogContext, obj resources.Object) bool {
	return this.classes.IsResponsibleFor(logger, obj)
}

func (this *state) Setup() {
	this.ownerupd = startOwnerUpdater(this.context, this.ownerresc)
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
		this.UpdateOwner(this.context.NewContext("owner", p.ObjectName().String()), p, true)
	}, processors)
	this.setupFor(&api.DNSEntry{}, "entries", func(e resources.Object) {
		p := dnsutils.DNSEntry(e)
		this.UpdateEntry(this.context.NewContext("entry", p.ObjectName().String()), p)
	}, processors)

	this.triggerStatistic()
	this.initialized = true
	this.context.Infof("setup done - starting reconciliation")
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
	this.setup.Start(this.context)
	this.setup = nil
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
	provider, _, err := this.lookupProvider(e.Object())
	return provider, err
}

type providerMatch struct {
	found DNSProvider
	match int
}

func (this *state) lookupProvider(e *dnsutils.DNSEntryObject) (DNSProvider, DNSProvider, error) {
	handleMatch := func(match *providerMatch, p *dnsProviderVersion, n int, err error) error {
		if match.match <= n {
			err2 := access.CheckAccessWithRealms(e, "use", p.Object(), this.realms)
			if err2 == nil {
				if match.match < n || (e.Status().Provider != nil && *e.Status().Provider == p.object.ObjectName().String()) {
					match.found = p
					match.match = n
				}
				return nil
			}
			if match.match == 0 {
				return err2
			}
		}
		return err
	}
	var err error
	validMatch := &providerMatch{}
	errorMatch := &providerMatch{}
	validMatchFallback := &providerMatch{}
	for _, p := range this.providers {
		n := p.Match(e.GetDNSName())
		if n > 0 {
			if p.IsValid() {
				err = handleMatch(validMatch, p, n, err)
			} else {
				err = handleMatch(errorMatch, p, n, err)
			}
		} else {
			n = p.MatchZone(e.GetDNSName())
			if n > 0 && p.IsValid() {
				handleMatch(validMatchFallback, p, n, nil)
			}
		}
	}
	if validMatch.found != nil {
		return validMatch.found, nil, nil
	}
	if errorMatch.found != nil {
		return errorMatch.found, nil, nil
	}
	return nil, validMatchFallback.found, err
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

func (this *state) addEntriesForZone(logger logger.LogContext, entries Entries, stale DNSNames, zone DNSHostedZone) (Entries, DNSNames, bool) {
	if entries == nil {
		entries = Entries{}
	}
	if stale == nil {
		stale = DNSNames{}
	}
	deleting := true // TODO check
	domain := zone.Domain()
	// fallback if no forwarded domains are reported
	nested := utils.NewStringSet()
	for _, z := range this.zones {
		if z.Domain() != domain && dnsutils.Match(z.Domain(), domain) {
			nested.Add(z.Domain())
		}
	}
loop:
	for dns, e := range this.dnsnames {
		if e.IsValid() {
			provider, fallback, err := this.lookupProvider(e.Object())
			if (provider == nil || !provider.IsValid()) && !e.IsDeleting() {
				if provider != nil {
					logger.Infof("no valid provider found for %q(%s found, but is not valid)", e.ObjectName(), provider.ObjectName())
				} else {
					if err != nil {
						logger.Infof("no valid provider found for %q(%s): %s", e.ObjectName(), dns, err)
					} else {
						logger.Infof("no valid provider found for %q(%s)", e.ObjectName(), dns)
					}
				}
				if fallback == nil || !fallback.IncludesZone(zone.Id()) {
					stale[e.DNSName()] = e
					continue
				}
			} else if !provider.IncludesZone(zone.Id()) {
				continue
			}
			if zone.Match(dns) > 0 {
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
				if e.KeepRecords() {
					stale[e.DNSName()] = e
				}
			}
		}
	}
	return entries, stale, deleting
}

func (this *state) GetZoneForEntry(e *Entry) string {
	if !e.IsValid() {
		return ""
	}
	provider, _, _ := this.lookupProvider(e.object)
	return this.GetProviderZoneForName(e.DNSName(), provider)
}

func (this *state) GetProviderZoneForName(name string, provider DNSProvider) string {
	this.lock.RLock()
	defer this.lock.RUnlock()

	found := this.getProviderZoneForName(name, provider)
	if found != nil {
		return found.Id()
	}
	return ""
}

func (this *state) getProviderZoneForName(hostname string, provider DNSProvider) *dnsHostedZone {
	zones := this.getZonesForName(hostname)
	return filterZoneByProvider(zones, provider)
}

// getZonesForName can return multiple zones in the case of private zones
func (this *state) getZonesForName(hostname string) []*dnsHostedZone {
	var found []*dnsHostedZone
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
				found = []*dnsHostedZone{zone}
			} else if length == len(name) {
				found = append(found, zone)
			}
		}
	}
	return found
}

func (this *state) triggerStatistic() {
	if this.context.IsReady() {
		this.context.EnqueueCommand(CMD_STATISTIC)
	} else {
		this.setup.AddCommand(CMD_STATISTIC)
	}
}

func (this *state) triggerHostedZone(name string) {
	cmd := CMD_HOSTEDZONE_PREFIX + name
	if this.context.IsReady() {
		this.context.EnqueueCommand(cmd)
	} else {
		this.setup.AddCommand(cmd)
	}
}

func (this *state) triggerKey(key resources.ClusterObjectKey) {
	if this.context.IsReady() {
		this.context.EnqueueKey(key)
	} else {
		this.setup.AddKey(key)
	}
}

func (this *state) DecodeZoneCommand(name string) string {
	if strings.HasPrefix(name, CMD_HOSTEDZONE_PREFIX) {
		return name[len(CMD_HOSTEDZONE_PREFIX):]
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
				zone = newDNSHostedZone(this.config.RescheduleDelay, z)
				this.zones[z.Id()] = zone
				logger.Infof("adding hosted zone %q (%s)", z.Id(), z.Domain())
				this.triggerHostedZone(zone.Id())
			}
			zone.update(z)

			if this.isProviderForZone(z.Id(), name) {
				if last != nil && (!new.included.Equals(last.included) || !new.excluded.Equals(last.excluded)) {
					modified = true
					logger.Infof("keeping provider %q for hosted zone %q (%s) with modified domain selection", name, z.Id(), z.Domain())
				} else {
					keeping = append(keeping, fmt.Sprintf("keeping provider %q for hosted zone %q (%s)", name, z.Id(), z.Domain()))
				}
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
			for zoneid, z := range old {
				if result[zoneid] == nil {
					modified = true
					this.removeProviderForZone(zoneid, name)
					logger.Infof("removing provider %q for hosted zone %q (%s)", name, z.Id(), z.Domain())
					if !this.hasProvidersForZone(zoneid) {
						logger.Infof("removing hosted zone %q (%s)", z.Id(), z.Domain())
						metrics.DeleteZone(z.Id())
						delete(this.zones, zoneid)
					}
				}
			}
		}
	}
	if modified {
		for _, m := range keeping {
			logger.Info(m)
		}
	}
	this.providerzones[name] = result
	return modified
}

func (this *state) RefineLogger(logger logger.LogContext, ptype string) logger.LogContext {
	if len(this.config.Enabled) > 1 && ptype != "" {
		logger = logger.NewContext("type", ptype)
	}
	return logger
}
