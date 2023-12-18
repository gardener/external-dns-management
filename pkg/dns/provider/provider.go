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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/selection"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	"github.com/gardener/external-dns-management/pkg/server/metrics"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

const ZoneCachePrefix = "zc-"

func (this DNSProviders) LookupFor(dns string) DNSProvider {
	var found DNSProvider
	match := -1
	for _, p := range this {
		n := p.Match(dns)
		if n > 0 {
			if match < n || match == n && found != nil && strings.Compare(p.AccountHash(), found.AccountHash()) < 0 {
				found = p
				match = n
			}
		}
	}
	return found
}

///////////////////////////////////////////////////////////////////////////////

type DNSAccount struct {
	*dnsutils.RateLimiter
	handler DNSHandler
	config  utils.Properties

	hash    string
	clients resources.ObjectNameSet
}

var (
	_ DNSHandler = &DNSAccount{}
	_ Metrics    = &DNSAccount{}
)

func NewDNSAccount(config utils.Properties, handler DNSHandler, hash string) *DNSAccount {
	return &DNSAccount{
		RateLimiter: dnsutils.NewRateLimiter(3*time.Second, 10*time.Minute),
		config:      config,
		handler:     handler,
		hash:        hash,
		clients:     resources.ObjectNameSet{},
	}
}

func (this *DNSAccount) AddGenericRequests(requestType string, n int) {
	metrics.AddRequests(this.handler.ProviderType(), this.hash, requestType, n, nil)
}

func (this *DNSAccount) AddZoneRequests(zoneID, requestType string, n int) {
	metrics.AddRequests(this.handler.ProviderType(), this.hash, requestType, n, &zoneID)
}

func (this *DNSAccount) ProviderType() string {
	return this.handler.ProviderType()
}

func (this *DNSAccount) Hash() string {
	return this.hash
}

func (this *DNSAccount) GetZones() (DNSHostedZones, error) {
	zones, err := this.handler.GetZones()
	if err == nil {
		zones = addObviousForwardedDomains(zones)
		this.Succeeded()
	} else {
		this.Failed()
	}
	return zones, err
}

func addObviousForwardedDomains(zones DNSHostedZones) DNSHostedZones {
	result := make(DNSHostedZones, len(zones))
	for i, zone := range zones {
		result[i] = zone
		if zone.IsPrivate() {
			continue
		}
		changed := false
		forwarded := zone.ForwardedDomains()
	otherloop:
		for j, other := range zones {
			if i == j || other.IsPrivate() || zone.Domain() == other.Domain() {
				continue
			}
			if Match(zone, other.Domain()) > 0 {
				for _, domain := range forwarded {
					if domain == other.Domain() {
						continue otherloop
					}
				}
				changed = true
				forwarded = append(forwarded, other.Domain())
			}
		}
		if changed {
			result[i] = CopyDNSHostedZone(zone, forwarded)
		}
	}
	return result
}

func (this *DNSAccount) GetZoneState(zone DNSHostedZone) (DNSZoneState, error) {
	state, err := this.handler.GetZoneState(zone)
	if err == nil {
		this.Succeeded()
	} else {
		this.Failed()
	}
	return state, err
}

func (this *DNSAccount) ReportZoneStateConflict(zone DNSHostedZone, err error) bool {
	return this.handler.ReportZoneStateConflict(zone, err)
}

func (this *DNSAccount) ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, reqs []*ChangeRequest) error {
	return this.handler.ExecuteRequests(logger, zone, state, reqs)
}

func (this *DNSAccount) MapTarget(t Target) Target {
	return this.handler.MapTarget(t)
}

func (this *DNSAccount) Release() {
	this.handler.Release()
}

type AccountCache struct {
	lock    sync.Mutex
	ttl     time.Duration
	cache   map[string]*DNSAccount
	options *FactoryOptions
}

func NewAccountCache(ttl time.Duration, opts *FactoryOptions) *AccountCache {
	return &AccountCache{
		ttl:   ttl,
		cache: map[string]*DNSAccount{},

		options: opts,
	}
}

func (this *AccountCache) Get(logger logger.LogContext, provider *dnsutils.DNSProviderObject, props utils.Properties, state *state) (*DNSAccount, error) {
	name := provider.ObjectName()
	hash := this.Hash(props, provider.Spec().Type, provider.Spec().ProviderConfig)
	this.lock.Lock()
	defer this.lock.Unlock()
	a := this.cache[hash]
	if a == nil {
		a = NewDNSAccount(props, nil, hash)
		syncPeriod := state.GetContext().GetPoolPeriod("dns")
		if syncPeriod == nil {
			return nil, fmt.Errorf("Pool dns not found")
		}
		cacheFactory := ZoneCacheFactory{
			context:               state.GetContext().GetContext(),
			logger:                logger,
			zonesTTL:              this.ttl,
			zoneStates:            state.zoneStates,
			disableZoneStateCache: !state.config.ZoneStateCaching,
		}

		cfg := DNSHandlerConfig{
			Context:          state.GetContext().GetContext(),
			Logger:           logger,
			Properties:       props,
			Config:           provider.Spec().ProviderConfig,
			DryRun:           state.GetConfig().Dryrun,
			ZoneCacheFactory: cacheFactory,
			Options:          this.options,
			Metrics:          a,
		}
		var err error
		a.handler, err = state.GetHandlerFactory().Create(provider.TypeCode(), &cfg)
		if err != nil {
			return nil, err
		}
		logger.Infof("creating account for %s (%s)", name, a.Hash())
		this.cache[hash] = a
	}
	old := len(a.clients)
	a.clients.Add(name)
	if old != len(a.clients) && old != 0 {
		logger.Infof("reusing account for %s (%s): %d client(s)", name, a.Hash(), len(a.clients))
	}
	metrics.ReportAccountProviders(provider.Spec().Type, a.Hash(), len(a.clients))
	return a, nil
}

var null = []byte{0}

func (this *AccountCache) Release(logger logger.LogContext, a *DNSAccount, name resources.ObjectName) {
	if a != nil {
		this.lock.Lock()
		defer this.lock.Unlock()

		a.clients.Remove(name)
		if len(a.clients) == 0 {
			logger.Infof("releasing account for %s (%s)", name, a.Hash())
			delete(this.cache, a.hash)
			metrics.DeleteAccount(a.ProviderType(), a.Hash())
			a.handler.Release()
		} else {
			logger.Infof("keeping account for %s (%s): %d client(s)", name, a.Hash(), len(a.clients))
		}
	}
}

func (this *AccountCache) Hash(props utils.Properties, ptype string, extension *runtime.RawExtension) string {
	keys := make([]string, len(props))
	i := 0
	h := sha256.New224()
	for k := range props {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := props[k]
		h.Write([]byte(k))
		h.Write(null)
		h.Write(([]byte(v)))
		h.Write(null)
	}

	if extension != nil {
		h.Write(extension.Raw)
	}
	h.Write(null)
	h.Write([]byte(ptype))
	return hex.EncodeToString(h.Sum(nil))
}

///////////////////////////////////////////////////////////////////////////////

type dnsProviderVersion struct {
	state *state

	object  *dnsutils.DNSProviderObject
	account *DNSAccount
	valid   bool

	defaultTTL int64

	secret      resources.ObjectName
	def_include utils.StringSet
	def_exclude utils.StringSet

	zones          DNSHostedZones
	included_zones utils.StringSet
	excluded_zones utils.StringSet

	included  utils.StringSet
	excluded  utils.StringSet
	rateLimit *api.RateLimit
}

var _ DNSProvider = &dnsProviderVersion{}

func (this *dnsProviderVersion) IsValid() bool {
	return this.valid
}

func (this *dnsProviderVersion) TypeCode() string {
	return this.object.TypeCode()
}

func (this *dnsProviderVersion) DefaultTTL() int64 {
	return this.defaultTTL
}

func (this *dnsProviderVersion) equivalentTo(v *dnsProviderVersion) bool {
	if this.account != v.account {
		return false
	}
	if !this.zones.EquivalentTo(v.zones) {
		return false
	}
	if !this.def_include.Equals(v.def_include) {
		return false
	}
	if !this.def_exclude.Equals(v.def_exclude) {
		return false
	}
	if !reflect.DeepEqual(this.defaultTTL, v.defaultTTL) {
		return false
	}
	if this.secret != nil && v.secret != nil && this.secret != v.secret {
		return false
	} else {
		if this.secret != v.secret {
			return false
		}
	}
	return true
}

func updateDNSProvider(logger logger.LogContext, state *state, provider *dnsutils.DNSProviderObject, last *dnsProviderVersion) (*dnsProviderVersion, reconcile.Status) {
	domsel := selection.PrepareSelection(provider.DNSProvider().Spec.Domains)
	this := &dnsProviderVersion{
		state:  state,
		object: provider,

		def_include: domsel.Include,
		def_exclude: domsel.Exclude,

		included: utils.StringSet{},
		excluded: utils.StringSet{},
	}

	if last != nil {
		this.included = last.included
		this.excluded = last.excluded
		this.included_zones = last.included_zones
		this.excluded_zones = last.excluded_zones
	} else {
		this.included = utils.NewStringSet(provider.Status().Domains.Included...)
		this.excluded = utils.NewStringSet(provider.Status().Domains.Excluded...)
		this.included_zones = utils.NewStringSet(provider.Status().Zones.Included...)
		this.excluded_zones = utils.NewStringSet(provider.Status().Zones.Excluded...)
	}

	if provider.Spec().DefaultTTL != nil {
		this.defaultTTL = *provider.Spec().DefaultTTL
	} else {
		this.defaultTTL = state.config.TTL
	}

	if last != nil && last.ObjectName() != this.ObjectName() {
		panic(fmt.Errorf("provider name mismatch %q<=>%q", last.ObjectName(), this.ObjectName()))
	}

	var props utils.Properties
	var err error

	ref := this.object.DNSProvider().Spec.SecretRef
	if ref == nil {
		return this, this.failed(logger, false, fmt.Errorf("no secret specified"), false)
	}

	localref := *ref
	ref = &localref
	if ref.Namespace == "" {
		ref.Namespace = provider.GetNamespace()
	}
	this.secret = resources.NewObjectName(ref.Namespace, ref.Name)
	props, _, err = state.GetContext().GetSecretPropertiesByRef(provider, ref)
	if err != nil {
		if errors.IsNotFound(err) {
			return this, this.failed(logger, false, fmt.Errorf("cannot get secret %s/%s for provider %s: %s",
				ref.Namespace, ref.Name, provider.Description(), err), false)
		}
		return this, this.failed(logger, false, fmt.Errorf("error reading secret for provider %q", provider.Description()), true)
	}

	this.account, err = state.GetDNSAccount(logger, provider, props)
	if err != nil {
		return this, this.failed(logger, false, err, true)
	}

	zones, err := this.account.GetZones()
	if err != nil {
		this.zones = nil
		return this, this.failed(logger, false, fmt.Errorf("cannot get hosted zones: %w", err), true)
	}
	if len(zones) == 0 {
		empty := utils.StringSet{}
		mod := this.object.SetSelection(empty, empty, &this.object.Status().Domains)
		mod = this.object.SetSelection(empty, empty, &this.object.Status().Zones) || mod
		return this, this.failedButRecheck(logger, fmt.Errorf("no hosted zones available in account"), mod)
	}

	results := selection.CalcZoneAndDomainSelection(provider.DNSProvider().Spec, toLightZones(zones))
	this.zones = fromLightZones(results.Zones)
	this.included = results.DomainSel.Include
	this.excluded = results.DomainSel.Exclude
	this.included_zones = results.ZoneSel.Include
	this.excluded_zones = results.ZoneSel.Exclude
	for _, warning := range results.Warnings {
		this.object.Eventf(corev1.EventTypeWarning, "reconcile", "%s", warning)
	}
	mod := this.object.SetSelection(this.included, this.excluded, &this.object.Status().Domains)
	mod = this.object.SetSelection(this.included_zones, this.excluded_zones, &this.object.Status().Zones) || mod
	if results.Error != "" {
		return this, this.failedButRecheck(logger, fmt.Errorf(results.Error), mod)
	}

	allForwardedDomains := utils.NewStringSet()
	for _, z := range this.zones {
		if z.Id().ProviderType == this.TypeCode() && this.included_zones.Contains(z.Id().ID) {
			if len(z.ForwardedDomains()) > 0 {
				allForwardedDomains.AddAll(z.ForwardedDomains())
			}
		}
	}
	for _, z := range this.zones {
		if z.Id().ProviderType == this.TypeCode() && this.included_zones.Contains(z.Id().ID) {
			for _, z2 := range this.zones {
				if z2.Id().ProviderType == this.TypeCode() && this.included_zones.Contains(z2.Id().ID) && z.Id() != z2.Id() {
					if z.Domain() == z2.Domain() {
						return this, this.failedButRecheck(logger, fmt.Errorf("duplicate zones %s(%s) and %s(%s)", z.Id(), z.Domain(), z2.Id(), z2.Domain()), mod)
					} else if dnsutils.Match(z2.Domain(), z.Domain()) && !allForwardedDomains.Contains(z2.Domain()) {
						return this, this.failedButRecheck(logger, fmt.Errorf("overlapping zones %s(%s) and %s(%s)", z.Id(), z.Domain(), z2.Id(), z2.Domain()), mod)
					}
				}
			}
		}
	}

	if last == nil || !this.included.Equals(last.included) || !this.excluded.Equals(last.excluded) {
		if len(this.included) > 0 {
			logger.Infof("  included domains: %s", this.included)
		}
		if len(this.excluded) > 0 {
			logger.Infof("  excluded domains: %s", this.excluded)
		}
	}

	if last == nil || !this.included_zones.Equals(last.included_zones) || !this.excluded_zones.Equals(last.excluded_zones) {
		if len(this.included_zones) > 0 {
			logger.Infof("  included zones: %s", this.included_zones)
		}
		if len(this.excluded_zones) > 0 {
			logger.Infof("  excluded zones: %s", this.excluded_zones)
		}
	}

	this.valid = true
	this.rateLimit = state.updateProviderRateLimiter(logger, provider)

	return this, this.succeeded(logger, mod)
}

func toLightZones(zones DNSHostedZones) []selection.LightDNSHostedZone {
	lzones := make([]selection.LightDNSHostedZone, len(zones))
	for i, z := range zones {
		lzones[i] = z
	}
	return lzones
}

func fromLightZones(lzones []selection.LightDNSHostedZone) DNSHostedZones {
	zones := make(DNSHostedZones, len(lzones))
	for i, lz := range lzones {
		zones[i] = lz.(DNSHostedZone)
	}
	return zones
}

func (this *dnsProviderVersion) AccountHash() string {
	return this.account.Hash()
}

func (this *dnsProviderVersion) ObjectName() resources.ObjectName {
	return this.object.ObjectName()
}

func (this *dnsProviderVersion) Object() resources.Object {
	return this.object
}

func (this *dnsProviderVersion) GetConfig() utils.Properties {
	return this.account.config
}

func (this *dnsProviderVersion) GetZones() DNSHostedZones {
	return this.zones
}

func (this *dnsProviderVersion) GetIncludedDomains() utils.StringSet {
	return this.included.Copy()
}

func (this *dnsProviderVersion) GetExcludedDomains() utils.StringSet {
	return this.excluded.Copy()
}

func (this *dnsProviderVersion) Match(dns string) int {
	ilen := dnsutils.MatchSet(dns, this.included)
	elen := dnsutils.MatchSet(dns, this.excluded)
	if ilen > elen {
		return ilen
	}
	return 0
}

func (this *dnsProviderVersion) MatchZone(dns string) int {
	for _, zone := range this.zones {
		ilen := zone.Match(dns)
		if ilen > 0 {
			return ilen
		}
	}
	return 0
}

func (this *dnsProviderVersion) MapTarget(t Target) Target {
	return this.account.MapTarget(t)
}

func (this *dnsProviderVersion) setError(modified bool, err error) error {
	modified = this.object.SetStateWithError(api.STATE_ERROR, err) || modified
	if modified {
		dnsutils.SetLastUpdateTime(&this.object.Status().LastUptimeTime)
		return this.object.UpdateStatus()
	}
	return nil
}

func (this *dnsProviderVersion) failed(logger logger.LogContext, modified bool, err error, temp bool) reconcile.Status {
	uerr := this.setError(modified, err)
	if uerr != nil {
		if temp {
			logger.Info(err)
		} else {
			logger.Error(err)
		}
		if errors.IsConflict(uerr) {
			return reconcile.Repeat(logger, fmt.Errorf("cannot update provider %q: %s", this.ObjectName(), uerr))
		}
		return reconcile.Delay(logger, uerr)
	}
	if !temp {
		if this.account != nil {
			this.account.Succeeded()
		}
		return reconcile.Failed(logger, err)
	}
	if this.account != nil {
		return reconcile.Recheck(logger, err, this.account.RateLimit())
	}
	return reconcile.Delay(logger, err)
}

func maxDuration(x, y time.Duration) time.Duration {
	if x < y {
		return y
	}
	return x
}

func (this *dnsProviderVersion) failedButRecheck(logger logger.LogContext, err error, modified bool) reconcile.Status {
	uerr := this.setError(modified, err)
	if uerr != nil {
		logger.Error(err)
		if errors.IsConflict(uerr) {
			return reconcile.Repeat(logger, fmt.Errorf("cannot update provider %q: %s", this.ObjectName(), uerr))
		}
	}
	return reconcile.Recheck(logger, err, maxDuration(this.state.config.CacheTTL, 30*time.Minute))
}

func (this *dnsProviderVersion) succeeded(logger logger.LogContext, modified bool) reconcile.Status {
	status := &this.object.DNSProvider().Status
	mod := resources.NewModificationState(this.object, modified)
	mod.AssureStringValue(&status.State, api.STATE_READY)
	mod.AssureStringPtrValue(&status.Message, "provider operational")
	mod.AssureInt64Value(&status.ObservedGeneration, this.object.DNSProvider().Generation)
	mod.AssureInt64PtrValue(&status.DefaultTTL, this.defaultTTL)
	assureRateLimit(mod, &status.RateLimit, this.rateLimit)
	if mod.IsModified() {
		dnsutils.SetLastUpdateTime(&this.object.Status().LastUptimeTime)
	}
	return reconcile.UpdateStatus(logger, mod)
}

func (this *dnsProviderVersion) GetZoneState(zone DNSHostedZone) (DNSZoneState, error) {
	return this.account.GetZoneState(zone)
}

func (this *dnsProviderVersion) ReportZoneStateConflict(zone DNSHostedZone, err error) bool {
	return this.account.ReportZoneStateConflict(zone, err)
}

func (this *dnsProviderVersion) ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, reqs []*ChangeRequest) error {
	return this.account.ExecuteRequests(logger, zone, state, reqs)
}

func (this *dnsProviderVersion) IncludesZone(zoneID dns.ZoneID) bool {
	return this.TypeCode() == zoneID.ProviderType && this.included_zones != nil && this.included_zones.Contains(zoneID.ID)
}

// HasEquivalentZone returns true for same provider specific zone id but different provider type and
// one zoneid has provider type "remote".
func (this *dnsProviderVersion) HasEquivalentZone(zoneID dns.ZoneID) bool {
	return this.TypeCode() != zoneID.ProviderType &&
		(this.TypeCode() == "remote" || zoneID.ProviderType == "remote") &&
		this.included_zones != nil && this.included_zones.Contains(zoneID.ID)
}

func (this *dnsProviderVersion) GetDedicatedDNSAccess() DedicatedDNSAccess {
	h, _ := this.account.handler.(DedicatedDNSAccess)
	return h
}
