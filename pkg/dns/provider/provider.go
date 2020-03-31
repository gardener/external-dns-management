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
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/gardener/external-dns-management/pkg/server/metrics"
	"k8s.io/apimachinery/pkg/runtime"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

const ZoneCachePrefix = "zc-"

func (this DNSProviders) LookupFor(dns string) DNSProvider {
	var found DNSProvider
	match := -1
	for _, p := range this {
		n := p.Match(dns)
		if n > 0 {
			if match < n {
				found = p
				match = n
			}
		}
	}
	return found
}

///////////////////////////////////////////////////////////////////////////////

type DNSAccount struct {
	handler DNSHandler
	config  utils.Properties

	hash    string
	clients resources.ObjectNameSet
}

var _ DNSHandler = &DNSAccount{}
var _ Metrics = &DNSAccount{}

func (this *DNSAccount) AddRequests(requestType string, n int) {
	metrics.AddRequests(this.handler.ProviderType(), this.hash, requestType, n)
}

func (this *DNSAccount) ProviderType() string {
	return this.handler.ProviderType()
}

func (this *DNSAccount) Hash() string {
	return this.hash
}

func (this *DNSAccount) GetZones() (DNSHostedZones, error) {
	return this.handler.GetZones()
}

func (this *DNSAccount) GetZoneState(zone DNSHostedZone) (DNSZoneState, error) {
	return this.handler.GetZoneState(zone)
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
	lock  sync.Mutex
	ttl   time.Duration
	dir   string
	cache map[string]*DNSAccount
}

func NewAccountCache(ttl time.Duration, dir string) *AccountCache {
	return &AccountCache{ttl: ttl, dir: dir, cache: map[string]*DNSAccount{}}
}

func (this *AccountCache) Get(logger logger.LogContext, provider *dnsutils.DNSProviderObject, props utils.Properties, state *state) (*DNSAccount, error) {
	var err error

	name := provider.ObjectName()
	hash := this.Hash(props, provider.Spec().Type, provider.Spec().ProviderConfig)
	this.lock.Lock()
	defer this.lock.Unlock()
	a := this.cache[hash]
	if a == nil {
		a = &DNSAccount{config: props, hash: hash, clients: resources.ObjectNameSet{}}
		syncPeriod := state.GetContext().GetPoolPeriod("dns")
		if syncPeriod == nil {
			return nil, fmt.Errorf("Pool dns not found")
		}
		persistDir := ""
		if this.dir != "" {
			persistDir = filepath.Join(this.dir, ZoneCachePrefix+hash)
		}
		cacheConfig := ZoneCacheConfig{
			context:               state.GetContext().GetContext(),
			logger:                logger,
			persistDir:            persistDir,
			zonesTTL:              this.ttl,
			stateTTL:              *syncPeriod,
			disableZoneStateCache: !state.config.ZoneStateCaching,
		}
		cfg := DNSHandlerConfig{
			Context:     state.GetContext().GetContext(),
			Logger:      logger,
			Properties:  props,
			Config:      provider.Spec().ProviderConfig,
			DryRun:      state.GetConfig().Dryrun,
			CacheConfig: cacheConfig,
			Metrics:     a,
		}
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

	secret      resources.ObjectName
	def_include utils.StringSet
	def_exclude utils.StringSet

	zones          DNSHostedZones
	included_zones utils.StringSet
	excluded_zones utils.StringSet

	included utils.StringSet
	excluded utils.StringSet
}

var _ DNSProvider = &dnsProviderVersion{}

func (this *dnsProviderVersion) IsValid() bool {
	return this.valid
}

func (this *dnsProviderVersion) equivalentTo(v *dnsProviderVersion) bool {
	if this.account != v.account {
		return false
	}
	if !this.zones.equivalentTo(v.zones) {
		return false
	}
	if !this.def_include.Equals(v.def_include) {
		return false
	}
	if !this.def_exclude.Equals(v.def_exclude) {
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

func prepareSelection(sel *api.DNSSelection) (includes, excludes utils.StringSet) {
	if sel != nil {
		includes = utils.NewStringSetByArray(sel.Include)
		excludes = utils.NewStringSetByArray(sel.Exclude)
	} else {
		includes = utils.StringSet{}
		excludes = utils.StringSet{}
	}
	return
}

func updateDNSProvider(logger logger.LogContext, state *state, provider *dnsutils.DNSProviderObject, last *dnsProviderVersion) (*dnsProviderVersion, reconcile.Status) {
	this := &dnsProviderVersion{
		state:  state,
		object: provider,

		def_include: utils.StringSet{},
		def_exclude: utils.StringSet{},

		included: utils.StringSet{},
		excluded: utils.StringSet{},
	}

	if last != nil && last.ObjectName() != this.ObjectName() {
		panic(fmt.Errorf("provider name mismatch %q<=>%q", last.ObjectName(), this.ObjectName()))
	}

	var props utils.Properties
	var err error

	ref := this.object.DNSProvider().Spec.SecretRef
	if ref != nil {
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
	} else {
		return this, this.failed(logger, false, fmt.Errorf("no secret specified"), false)
	}

	this.account, err = state.GetDNSAccount(logger, provider, props)
	if err != nil {
		return this, this.failed(logger, false, fmt.Errorf("%s", err), true)
	}

	this.def_include, this.def_exclude = prepareSelection(provider.DNSProvider().Spec.Domains)
	zones, err := this.account.GetZones()
	if err != nil {
		this.zones = nil
		return this, this.failed(logger, false, fmt.Errorf("cannot get zones: %s", err), true)
	}

	zinc, zexc := prepareSelection(provider.DNSProvider().Spec.Zones)
	included_zones := utils.StringSet{}
	excluded_zones := utils.StringSet{}
	this.zones = DNSHostedZones{}
	if len(zinc) > 0 {
		for _, z := range zones {
			if zinc.Contains(z.Id()) {
				included_zones.Add(z.Id())
				this.zones = append(this.zones, z)
			} else {
				excluded_zones.Add(z.Id())
			}
		}
	} else {
		for _, z := range zones {
			included_zones.Add(z.Id())
			this.zones = append(this.zones, z)
		}
	}
	if len(zexc) > 0 {
		for i, z := range this.zones {
			if zexc.Contains(z.Id()) {
				this.zones = append(this.zones[:i], this.zones[i+1:]...)
				included_zones.Remove(z.Id())
				excluded_zones.Add(z.Id())
			}
		}
	}

	if len(zones) > 0 && len(this.zones) == 0 {
		return this, this.failedButRecheck(logger, fmt.Errorf("no zone available in account matches zone filter"))
	}

	included, err := filterByZones(this.def_include, this.zones)
	if err != nil {
		this.object.Eventf(corev1.EventTypeWarning, "reconcile", "%s", err)
	}
	excluded, err := filterByZones(this.def_exclude, this.zones)
	if err != nil {
		this.object.Eventf(corev1.EventTypeWarning, "reconcile", "%s", err)
	}

	if len(this.def_include) == 0 {
		if len(this.zones) == 0 {
			return this, this.failedButRecheck(logger, fmt.Errorf("no hosted zones found"))
		}
		for _, z := range this.zones {
			included.Add(z.Domain())
		}
	} else {
		if len(included) == 0 {
			return this, this.failedButRecheck(logger, fmt.Errorf("no domain matching hosting zones"))
		}
	}

	if last == nil || !included.Equals(last.included) || !excluded.Equals(last.excluded) {
		if len(included) > 0 {
			logger.Infof("  included domains: %s", included)
		}
		if len(excluded) > 0 {
			logger.Infof("  excluded domains: %s", excluded)
		}
	}
	this.included = included
	this.excluded = excluded

outer:
	for _, zone := range this.zones {
		for i := range included {
			if dnsutils.Match(i, zone.Domain()) {
				continue outer
			}
		}
		included_zones.Remove(zone.Id())
		excluded_zones.Add(zone.Id())
	}
	if last == nil || !included_zones.Equals(last.included_zones) || !excluded_zones.Equals(last.excluded_zones) {
		if len(included_zones) > 0 {
			logger.Infof("  included zones: %s", included_zones)
		}
		if len(excluded_zones) > 0 {
			logger.Infof("  excluded zones: %s", excluded_zones)
		}
	}
	this.included_zones = included_zones
	this.excluded_zones = excluded_zones

	this.valid = true
	mod := this.object.SetSelection(included, excluded, &this.object.Status().Domains)
	mod = this.object.SetSelection(included_zones, excluded_zones, &this.object.Status().Zones) || mod
	return this, this.succeeded(logger, mod)
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
	return -1
}

func (this *dnsProviderVersion) MapTarget(t Target) Target {
	return this.account.MapTarget(t)
}

func (this *dnsProviderVersion) setError(modified bool, err error) error {
	modified = this.object.SetSelection(utils.StringSet{}, utils.StringSet{}, &this.object.Status().Domains) || modified
	modified = this.object.SetSelection(utils.StringSet{}, utils.StringSet{}, &this.object.Status().Zones) || modified
	modified = this.object.SetState(api.StateError, err.Error()) || modified
	if modified {
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
		return reconcile.Failed(logger, uerr)
	}
	if !temp {
		return reconcile.Failed(logger, err)
	}
	return reconcile.Delay(logger, err)
}

func maxDuration(x, y time.Duration) time.Duration {
	if x < y {
		return y
	}
	return x
}

func (this *dnsProviderVersion) failedButRecheck(logger logger.LogContext, err error) reconcile.Status {
	uerr := this.setError(false, err)
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
	mod.AssureStringValue(&status.State, api.StateReady)
	mod.AssureStringPtrValue(&status.Message, "provider operational")
	mod.AssureInt64Value(&status.ObservedGeneration, this.object.DNSProvider().Generation)
	return reconcile.UpdateStatus(logger, mod.UpdateStatus())
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
