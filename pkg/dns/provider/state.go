// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/access"
	"github.com/gardener/controller-manager-library/pkg/utils"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	"github.com/gardener/external-dns-management/pkg/server/remote/embed"
)

type ZonedDNSSetName struct {
	dns.DNSSetName
	ZoneID dns.ZoneID
}

func (z ZonedDNSSetName) String() string {
	return fmt.Sprintf("%s[%s]", z.DNSSetName, z.ZoneID)
}

type ZonedDNSSetNames map[ZonedDNSSetName]*Entry

type zoneReconciliation struct {
	zone         *dnsHostedZone
	providers    DNSProviders
	entries      Entries
	equivEntries dns.DNSNameSet
	ownership    dns.Ownership
	stale        ZonedDNSSetNames
	dedicated    bool
	deleting     bool
	fhandler     FinalizerHandler
	dnsTicker    *Ticker
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

func (this *setup) Start(pctx ProviderContext) {
	this.lock.Lock()
	defer this.lock.Unlock()
	for c := range this.pending {
		pctx.Infof("trigger %s", c)
		_ = pctx.EnqueueCommand(c)
	}

	for key := range this.pendingKeys {
		pctx.Infof("trigger key %s/%s", key.Namespace(), key.Name())
		_ = pctx.EnqueueKey(key)
	}
	this.pending = nil
	this.pendingKeys = nil
}

////////////////////////////////////////////////////////////////////////////////

type state struct {
	lock        sync.RWMutex
	startupTime time.Time

	setup *setup

	context   ProviderContext
	ownerresc resources.Interface
	ownerupd  chan OwnerCounts

	secretresc resources.Interface

	classes *controller.Classes
	config  Config

	realms access.RealmTypes

	accountCache *AccountCache
	ownerCache   *OwnerCache
	zoneStates   *zoneStates

	foreign         map[resources.ObjectName]*foreignProvider
	providers       map[resources.ObjectName]*dnsProviderVersion
	deleting        map[resources.ObjectName]*dnsProviderVersion
	secrets         map[resources.ObjectName]resources.ObjectNameSet
	zones           map[dns.ZoneID]*dnsHostedZone
	zoneproviders   map[dns.ZoneID]resources.ObjectNameSet
	providerzones   map[resources.ObjectName]map[dns.ZoneID]*dnsHostedZone
	providersecrets map[resources.ObjectName]resources.ObjectName
	zonePolicies    map[string]*dnsHostedZonePolicy
	zoneStateTTL    atomic.Value

	entries         Entries
	outdated        *synchronizedEntries
	blockingEntries map[resources.ObjectName]time.Time

	providerRateLimiter map[resources.ObjectName]*rateLimiterData
	prlock              sync.RWMutex

	dnsnames   ZonedDNSSetNames
	references *References

	initialized bool

	dnsTicker *Ticker

	lookupProcessor *lookupProcessor

	providerEventListeners []ProviderEventListener
}

type rateLimiterData struct {
	api.RateLimit
	rateLimiter flowcontrol.RateLimiter
	lastAccept  atomic.Value
}

func NewDNSState(pctx ProviderContext, ownerresc, secretresc resources.Interface, classes *controller.Classes, config Config) *state {
	pctx.Infof("responsible for classes:     %s (%s)", classes, classes.Main())
	pctx.Infof("availabled providers types   %s", config.Factory.TypeCodes())
	pctx.Infof("enabled providers types:     %s", config.EnabledTypes)
	pctx.Infof("using default ttl:           %d", config.TTL)
	pctx.Infof("using identifier:            %s", config.Ident)
	pctx.Infof("dry run mode:                %t", config.Dryrun)
	pctx.Infof("reschedule delay:            %v", config.RescheduleDelay)
	pctx.Infof("zone cache ttl for zones:    %v", config.CacheTTL)
	pctx.Infof("disable zone state caching:  %t", !config.ZoneStateCaching)
	pctx.Infof("disable DNS name validation:  %t", config.DisableDNSNameValidation)
	if config.RemoteAccessConfig != nil {
		pctx.Infof("remote access server port: %d", config.RemoteAccessConfig.Port)
	}

	realms := access.RealmTypes{"use": access.NewRealmType(dns.REALM_ANNOTATION)}

	return &state{
		setup:               newSetup(),
		classes:             classes,
		context:             pctx,
		ownerresc:           ownerresc,
		secretresc:          secretresc,
		config:              config,
		realms:              realms,
		accountCache:        NewAccountCache(config.CacheTTL, config.Options),
		ownerCache:          NewOwnerCache(pctx, &config),
		foreign:             map[resources.ObjectName]*foreignProvider{},
		providers:           map[resources.ObjectName]*dnsProviderVersion{},
		deleting:            map[resources.ObjectName]*dnsProviderVersion{},
		zones:               map[dns.ZoneID]*dnsHostedZone{},
		secrets:             map[resources.ObjectName]resources.ObjectNameSet{},
		zoneproviders:       map[dns.ZoneID]resources.ObjectNameSet{},
		providerzones:       map[resources.ObjectName]map[dns.ZoneID]*dnsHostedZone{},
		providersecrets:     map[resources.ObjectName]resources.ObjectName{},
		zonePolicies:        map[string]*dnsHostedZonePolicy{},
		entries:             Entries{},
		outdated:            newSynchronizedEntries(),
		blockingEntries:     map[resources.ObjectName]time.Time{},
		dnsnames:            map[ZonedDNSSetName]*Entry{},
		references:          NewReferenceCache(),
		providerRateLimiter: map[resources.ObjectName]*rateLimiterData{},
	}
}

func (this *state) IsResponsibleFor(logger logger.LogContext, obj resources.Object) bool {
	return this.classes.IsResponsibleFor(logger, obj)
}

func (this *state) Setup() error {
	syncPeriod := this.context.GetPoolPeriod(DNS_POOL)
	if syncPeriod == nil {
		return fmt.Errorf("Pool %s not found", DNS_POOL)
	}
	this.zoneStates = newZoneStates(this.CreateStateTTLGetter(*syncPeriod))
	this.dnsTicker = NewTicker(this.context.GetPool(DNS_POOL).Tick)
	this.ownerupd = startOwnerUpdater(this.context, this.ownerresc)
	processors, err := this.context.GetIntOption(OPT_SETUP)
	if err != nil || processors <= 0 {
		processors = 5
	}

	this.lookupProcessor = newLookupProcessor(
		this.context.NewContext("sub", "lookupProcessor"),
		this.context,
		max(processors/5, 2),
		15*time.Second,
		this.context.GetCluster(TARGET_CLUSTER).GetId(),
		defaultLookupMetrics{},
	)

	if this.config.RemoteAccessConfig != nil {
		secret := &corev1.Secret{}
		_, err := this.secretresc.GetInto(this.config.RemoteAccessConfig.SecretName, secret)
		if err != nil {
			secret = nil
			this.context.Infof("remote access server secret not available: %s", err)
		}
		err = this.startRemoteAccessServer(secret)
		if err != nil {
			return fmt.Errorf("startRemoteAccessServer failed with: %w", err)
		}
	}

	this.context.Infof("using %d parallel workers for initialization", processors)
	if err := this.setupFor(&api.DNSProvider{}, "providers", func(e resources.Object) error {
		p := dnsutils.DNSProvider(e)
		if this.GetHandlerFactory().IsResponsibleFor(p) {
			this.UpdateProvider(this.context.NewContext("provider", p.ObjectName().String()), p)
		}
		return nil
	}, processors); err != nil {
		return err
	}
	if err := this.setupFor(&api.DNSOwner{}, "owners", func(e resources.Object) error {
		p := dnsutils.DNSOwner(e)
		this.UpdateOwner(this.context.NewContext("owner", p.ObjectName().String()), p, true)
		return nil
	}, processors); err != nil {
		return err
	}
	if err := this.setupFor(&api.DNSEntry{}, "entries", func(e resources.Object) error {
		p := dnsutils.DNSEntry(e)
		this.UpdateEntry(this.context.NewContext("entry", p.ObjectName().String()), p)
		return nil
	}, processors); err != nil {
		return err
	}

	this.triggerStatistic()
	this.initialized = true
	this.context.Infof("setup done - starting reconciliation")
	return nil
}

func (this *state) startRemoteAccessServer(secret *corev1.Secret) error {
	this.context.Infof("starting RemoteAccessServer")
	server, err := embed.StartDNSHandlerServer(this.context, this.config.RemoteAccessConfig)
	if err != nil {
		return err
	}
	this.config.RemoteAccessConfig.ServerSecretProvider.UpdateSecret(secret)

	listener, ok := server.(ProviderEventListener)
	if !ok {
		return fmt.Errorf("cannot cast server to ProviderEventListener")
	}
	this.providerEventListeners = append(this.providerEventListeners, listener)

	return nil
}

func (this *state) setupFor(obj runtime.Object, msg string, exec func(resources.Object) error, processors int) error {
	this.context.Infof("### setup %s", msg)
	res, err := this.context.GetByExample(obj)
	if err != nil {
		return err
	}
	list, err := res.ListCached(labels.Everything())
	if err != nil {
		return err
	}
	return dnsutils.ProcessElements(list, func(e resources.Object) error {
		if !this.IsResponsibleFor(this.context, e) {
			return nil
		}
		return exec(e)
	}, processors)
}

func (this *state) Start() {
	this.setup.Start(this.context)
	this.setup = nil
	this.startupTime = time.Now()
	go this.lookupProcessor.Run(this.context.GetContext())
}

func (this *state) HasFinalizer(obj resources.Object) bool {
	return this.context.HasFinalizer(obj)
}

func (this *state) SetFinalizer(obj resources.Object) error {
	return this.context.SetFinalizer(obj)
}

func (this *state) RemoveFinalizer(obj resources.Object) error {
	if len(obj.GetFinalizers()) == 0 {
		return nil
	}
	return this.context.RemoveFinalizer(obj)
}

func (this *state) GetContext() ProviderContext {
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

func (this *state) GetProvidersForZone(zoneid dns.ZoneID) DNSProviders {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.getProvidersForZone(zoneid)
}

func (this *state) HasProvidersForZone(zoneid dns.ZoneID) bool {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.hasProvidersForZone(zoneid)
}

func (this *state) hasProvidersForZone(zoneid dns.ZoneID) bool {
	return len(this.zoneproviders[zoneid]) > 0
}

func (this *state) isProviderForZone(zoneid dns.ZoneID, p resources.ObjectName) bool {
	set := this.zoneproviders[zoneid]
	return set != nil && set.Contains(p)
}

func (this *state) getProvidersForZone(zoneid dns.ZoneID) DNSProviders {
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

func (this *state) addProviderForZone(zoneid dns.ZoneID, p resources.ObjectName) {
	set := this.zoneproviders[zoneid]
	if set == nil {
		set = resources.ObjectNameSet{}
		this.zoneproviders[zoneid] = set
	}
	set.Add(p)
}

func (this *state) removeProviderForZone(zoneid dns.ZoneID, p resources.ObjectName) {
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
				_ = handleMatch(validMatchFallback, p, n, nil)
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

	p, ok := this.providers[name]
	if !ok {
		return nil
	}
	return p
}

func (this *state) GetZonesForProvider(name resources.ObjectName) dnsHostedZones {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return copyZones(this.providerzones[name])
}

func (this *state) GetEntriesForZone(logger logger.LogContext, zoneid dns.ZoneID) (Entries, ZonedDNSSetNames, bool) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	entries := Entries{}
	zone := this.zones[zoneid]
	if zone != nil {
		entries, _, stale, deleting := this.addEntriesForZone(logger, entries, ZonedDNSSetNames{}, zone)
		return entries, stale, deleting
	}
	return entries, nil, false
}

func (this *state) addEntriesForZone(
	logger logger.LogContext,
	entries Entries,
	stale ZonedDNSSetNames,
	zone DNSHostedZone,
) (
	Entries,
	dns.DNSNameSet,
	ZonedDNSSetNames,
	bool,
) {
	if entries == nil {
		entries = Entries{}
	}
	if stale == nil {
		stale = ZonedDNSSetNames{}
	}
	equivEntries := dns.DNSNameSet{}
	deleting := true // TODO check
	domain := zone.Domain()
	// fallback if no forwarded domains are reported
	nested := utils.NewStringSet()
	for _, z := range this.zones {
		if z.Domain() != domain && dnsutils.Match(z.Domain(), domain) {
			nested.Add(z.Domain())
		}
	}
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
					stale[e.ZonedDNSName()] = e
					continue
				}
			} else if provider == nil {
				continue
			} else if !provider.IncludesZone(zone.Id()) {
				if provider.HasEquivalentZone(zone.Id()) && e.IsActive() && !forwarded(nested, dns.DNSName) {
					equivEntries.Add(dns.DNSSetName)
				}
				continue
			}
			if dns.ZoneID == zone.Id() && zone.Match(dns.DNSName) > 0 && !forwarded(nested, dns.DNSName) {
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
					stale[e.ZonedDNSName()] = e
				}
			}
		}
	}
	return entries, equivEntries, stale, deleting
}

func (this *state) GetZoneForEntry(e *Entry) *dns.ZoneID {
	if !e.IsValid() {
		return nil
	}
	provider, _, _ := this.lookupProvider(e.object)
	return this.GetProviderZoneForName(e.DNSName(), provider)
}

func (this *state) GetProviderZoneForName(name string, provider DNSProvider) *dns.ZoneID {
	this.lock.RLock()
	defer this.lock.RUnlock()

	found := this.getProviderZoneForName(name, provider)
	if found != nil {
		z := found.Id()
		return &z
	}
	return nil
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
		_ = this.context.EnqueueCommand(CMD_STATISTIC)
	} else {
		this.setup.AddCommand(CMD_STATISTIC)
	}
}

func (this *state) triggerHostedZone(zoneid dns.ZoneID) {
	cmd := CMD_HOSTEDZONE_PREFIX + zoneid.ProviderType + ":" + zoneid.ID
	if this.context.IsReady() {
		_ = this.context.EnqueueCommand(cmd)
	} else {
		this.setup.AddCommand(cmd)
	}
}

func (this *state) triggerKey(key resources.ClusterObjectKey) {
	if this.context.IsReady() {
		_ = this.context.EnqueueKey(key)
	} else {
		this.setup.AddKey(key)
	}
}

func (this *state) DecodeZoneCommand(name string) *dns.ZoneID {
	if strings.HasPrefix(name, CMD_HOSTEDZONE_PREFIX) {
		parts := strings.SplitN(name[len(CMD_HOSTEDZONE_PREFIX):], ":", 2)
		if len(parts) == 2 {
			zoneid := dns.NewZoneID(parts[0], parts[1])
			return &zoneid
		}
	}
	return nil
}

func (this *state) updateZones(logger logger.LogContext, last, new *dnsProviderVersion) bool {
	var name resources.ObjectName
	keeping := []string{}
	modified := false
	result := map[dns.ZoneID]*dnsHostedZone{}
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
				this.triggerAllZonePolicies()
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

		for zoneid, z := range this.providerzones[name] {
			if result[zoneid] == nil {
				modified = true
				this.removeProviderForZone(zoneid, name)
				logger.Infof("removing provider %q for hosted zone %q (%s)", name, z.Id(), z.Domain())
				if !this.hasProvidersForZone(zoneid) {
					logger.Infof("removing hosted zone %q (%s)", z.Id(), z.Domain())
					this.deleteZone(zoneid)
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
	if len(this.config.EnabledTypes) > 1 && ptype != "" {
		logger = logger.NewContext("type", ptype)
	}
	return logger
}

func (this *state) tryAcceptProviderRateLimiter(logger logger.LogContext, entry *Entry) (bool, time.Duration) {
	delay := 0 * time.Second
	if entry.providername == nil {
		logger.Infof("missing providername for entry %s", entry.ObjectName())
		return true, delay
	}
	this.prlock.Lock()
	defer this.prlock.Unlock()

	rt := this.providerRateLimiter[entry.providername]
	if rt == nil {
		// not rate limited
		return true, delay
	}
	accepted := rt.rateLimiter.TryAccept()
	if accepted {
		rt.lastAccept.Store(time.Now())
	} else {
		delay = time.Duration(86400/rt.RequestsPerDay) * time.Second
		value := rt.lastAccept.Load()
		if value != nil {
			lastAccept := value.(time.Time)
			delay -= time.Since(lastAccept)
		}
		if delay < 100*time.Millisecond {
			delay = 100 * time.Millisecond
		}
	}
	return accepted, delay
}

func (this *state) ObjectUpdated(key resources.ClusterObjectKey) {
	this.context.Infof("requeue %s because of change in annotation resource", key)
	_ = this.context.EnqueueKey(key)
}

func forwarded(nested utils.StringSet, dnsname string) bool {
	for excl := range nested {
		if dnsutils.Match(dnsname, excl) {
			return true
		}
	}
	return false
}
