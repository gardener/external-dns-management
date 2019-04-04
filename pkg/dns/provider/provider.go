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

func (this DNSProviders) LookupFor(dns string) DNSProvider {
	var found DNSProvider
	match := -1
	for _, p := range this {
		n := p.Match(dns)
		if n > 0 {
			if match < n {
				found = p
			}
		}
	}
	return found
}

type dnsProvider struct {
	*dnsProviderVersion
}

func newProvider() *dnsProvider {
	return &dnsProvider{}
}

///////////////////////////////////////////////////////////////////////////////

type DNSAccount struct {
	lock    sync.Mutex
	handler DNSHandler
	config  utils.Properties

	hash    string
	clients resources.ObjectNameSet
	metrics Metrics

	ttl   time.Duration
	next  time.Time
	err   error
	zones DNSHostedZones
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
	this.lock.Lock()
	defer this.lock.Unlock()
	now := time.Now()
	if now.After(this.next) {
		this.zones, this.err = this.handler.GetZones()
		if this.err != nil {
			this.next = now.Add(this.ttl / 2)
		} else {
			this.next = now.Add(this.ttl)
		}
	}
	return this.zones, this.err
}

func (this *DNSAccount) GetZoneState(zone DNSHostedZone) (DNSZoneState, error) {
	return this.handler.GetZoneState(zone)
}

func (this *DNSAccount) ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, reqs []*ChangeRequest) error {
	return this.handler.ExecuteRequests(logger, zone, state, reqs)
}

type AccountCache struct {
	lock  sync.Mutex
	ttl   time.Duration
	cache map[string]*DNSAccount
}

func NewAccountCache(ttl time.Duration) *AccountCache {
	return &AccountCache{ttl: ttl, cache: map[string]*DNSAccount{}}
}

func (this *AccountCache) Get(logger logger.LogContext, provider *dnsutils.DNSProviderObject, props utils.Properties, state *state) (*DNSAccount, error) {
	var err error

	name := provider.ObjectName()
	h := this.Hash(props, provider.Spec().ProviderConfig)
	this.lock.Lock()
	defer this.lock.Unlock()
	a := this.cache[h]
	if a == nil {
		cfg := DNSHandlerConfig{
			Context:    state.GetController().GetContext(),
			Properties: props,
			Config:     provider.Spec().ProviderConfig,
			DryRun:     state.GetConfig().Dryrun,
		}
		a = &DNSAccount{ttl: this.ttl, config: props, hash: h, clients: resources.ObjectNameSet{}}
		a.handler, err = state.GetHandlerFactory().Create(logger, &cfg, a)
		if err != nil {
			return nil, err
		}
		logger.Infof("creating account for %s (%s)", name, a.Hash())
		this.cache[h] = a
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
	this.lock.Lock()
	defer this.lock.Unlock()

	a.clients.Remove(name)
	if len(a.clients) == 0 {
		logger.Infof("releasing account for %s (%s)", name, a.Hash())
		delete(this.cache, a.hash)
		metrics.DeleteAccount(a.ProviderType(), a.Hash())
	} else {
		logger.Infof("keeping account for %s (%s): %d client(s)", name, a.Hash(), len(a.clients))
	}
}

func (this *AccountCache) Hash(props utils.Properties, extension *runtime.RawExtension) string {
	keys := make([]string, len(props))
	i := 0
	h := sha256.New()
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

	zones DNSHostedZones

	included utils.StringSet
	excluded utils.StringSet
}

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
		props, _, err = resources.GetCachedSecretPropertiesByRef(provider, ref)
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

	dspec := provider.DNSProvider().Spec.Domains
	if dspec != nil {
		this.def_include = utils.NewStringSetByArray(dspec.Include)
		this.def_exclude = utils.NewStringSetByArray(dspec.Exclude)
	} else {
		this.def_include = utils.StringSet{}
		this.def_exclude = utils.StringSet{}
	}

	this.zones, err = this.account.GetZones()
	if err != nil {
		return this, this.failed(logger, false, fmt.Errorf("cannot get zones: %s", err), true)
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
			return this, this.failed(logger, false, fmt.Errorf("no hosted zones found"), false)
		}
		for _, z := range this.zones {
			included.Add(z.Domain())
		}
	} else {
		if len(included) == 0 {
			return this, this.failed(logger, false, fmt.Errorf("no domain matching hosting zones"), false)

		}
	}
	for _, zone := range this.zones {
		for _, sub := range zone.ForwardedDomains() {
			for i := range included {
				if dnsutils.Match(sub, i) {
					excluded.Add(sub)
				}
			}
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

	this.valid = true
	return this, this.succeeded(logger, this.object.SetDomains(included, excluded))
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
	return ilen - elen
}

func (this *dnsProviderVersion) setError(modified bool, err error) error {
	m1 := this.object.SetDomains(utils.StringSet{}, utils.StringSet{})
	m2 := this.object.SetState(api.STATE_ERROR, err.Error())
	modified = modified || m1 || m2
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

func (this *dnsProviderVersion) succeeded(logger logger.LogContext, modified bool) reconcile.Status {
	status := &this.object.DNSProvider().Status
	mod := resources.NewModificationState(this.object, modified)
	mod.AssureStringValue(&status.State, api.STATE_READY)
	mod.AssureStringPtrValue(&status.Message, "provider operational")
	mod.AssureInt64Value(&status.ObservedGeneration, this.object.DNSProvider().Generation)
	return reconcile.UpdateStatus(logger, mod.UpdateStatus())
}

func (this *dnsProviderVersion) GetZoneState(zone DNSHostedZone) (DNSZoneState, error) {
	return this.account.GetZoneState(zone)
}

func (this *dnsProviderVersion) ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, reqs []*ChangeRequest) error {
	return this.account.ExecuteRequests(logger, zone, state, reqs)
}
