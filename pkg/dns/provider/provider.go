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
	"github.com/gardener/external-dns-management/pkg/dns"
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

type dnsProviderVersion struct {
	state DNSState

	object  *dnsutils.DNSProviderObject
	handler DNSHandler

	config      utils.Properties
	secret      resources.ObjectName
	def_include utils.StringSet
	def_exclude utils.StringSet

	zoneinfos DNSHostedZoneInfos

	included utils.StringSet
	excluded utils.StringSet
}

func (this *dnsProviderVersion) equivalentTo(v *dnsProviderVersion) bool {
	if !this.zoneinfos.equivalentTo(v.zoneinfos) {
		return false
	}
	if !this.def_include.Equals(v.def_include) {
		return false
	}
	if !this.def_exclude.Equals(v.def_exclude) {
		return false
	}
	if !this.config.Equals(v.config) {
		return false
	}
	if this.secret != nil && v.secret != nil && this.secret != v.secret {
		return false
	} else {
		if this.secret != v.secret {
			return false
		}
	}
	if this.modified(v.object.DNSProvider().Spec.ProviderConfig) {
		return false
	}
	return true
}

func updateDNSProvider(logger logger.LogContext, state DNSState, provider *dnsutils.DNSProviderObject, last *dnsProviderVersion) (*dnsProviderVersion, reconcile.Status) {
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
		props, _, err = resources.GetSecretPropertiesByRef(provider, ref)
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

	this.config = props

	if last == nil || !last.config.Equals(props) || this.modified(provider.DNSProvider().Spec.ProviderConfig) {
		cfg := DNSHandlerConfig{
			Context:    this.state.GetController().GetContext(),
			Properties: props,
			Config:     provider.DNSProvider().Spec.ProviderConfig,
			DryRun:     state.GetConfig().Dryrun,
		}
		this.handler, err = state.GetHandlerFactory().Create(logger, &cfg)
		if err != nil {
			return nil, reconcile.Delay(logger, err)
		}
	} else {
		this.handler = last.handler
	}

	dspec := provider.DNSProvider().Spec.Domains
	if dspec != nil {
		this.def_include = utils.NewStringSetByArray(dspec.Include)
		this.def_exclude = utils.NewStringSetByArray(dspec.Exclude)
	} else {
		this.def_include = utils.StringSet{}
		this.def_exclude = utils.StringSet{}
	}

	this.zoneinfos, err = this.handler.GetZones()
	if err != nil {
		return nil, this.failed(logger, false, fmt.Errorf("cannot get zones: %s", err), true)
	}

	included, err := filterByZones(this.def_include, this.zoneinfos)
	if err != nil {
		this.object.Eventf(corev1.EventTypeWarning, "reconcile", "%s", err)
	}
	excluded, err := filterByZones(this.def_exclude, this.zoneinfos)
	if err != nil {
		this.object.Eventf(corev1.EventTypeWarning, "reconcile", "%s", err)
	}

	if len(this.def_include) == 0 {
		if len(this.zoneinfos) == 0 {
			return nil, this.failed(logger, false, fmt.Errorf("no hosted zones found"), false)
		}
		for _, z := range this.zoneinfos {
			included.Add(z.Domain)
		}
	} else {
		if len(included) == 0 {
			return nil, this.failed(logger, false, fmt.Errorf("no domain matching hosting zones"), false)

		}
	}
	for _, zone := range this.zoneinfos {
		for _, sub := range zone.Forwarded {
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

	return this, this.succeeded(logger, this.object.SetDomains(included, excluded))
}

func (this *dnsProviderVersion) ObjectName() resources.ObjectName {
	return this.object.ObjectName()
}

func (this *dnsProviderVersion) Object() resources.Object {
	return this.object
}

func (this *dnsProviderVersion) GetConfig() utils.Properties {
	return this.config
}

func (this *dnsProviderVersion) GetZoneInfos() DNSHostedZoneInfos {
	return this.zoneinfos
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

func (this *dnsProviderVersion) modified(new *runtime.RawExtension) bool {
	config := this.object.DNSProvider().Spec.ProviderConfig
	if config == new {
		return true
	}
	if config == nil || new == nil {
		return false
	}
	if len(config.Raw) != len(new.Raw) {
		return false
	}
	for i, b := range config.Raw {
		if new.Raw[i] != b {
			return false
		}
	}
	return true
}

func (this *dnsProviderVersion) setError(modified bool, err error) error {
	modified = modified || this.object.SetDomains(utils.StringSet{}, utils.StringSet{})
	modified = modified || this.object.SetState(api.STATE_ERROR, err.Error())
	if modified {
		return this.object.Update()
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
	return reconcile.UpdateStatus(logger, mod.Update())
}

func (this *dnsProviderVersion) GetDNSSets(zoneid string) (dns.DNSSets, error) {
	return this.handler.GetDNSSets(zoneid)
}

func (this *dnsProviderVersion) ExecuteRequests(logger logger.LogContext, zone DNSHostedZoneInfo, reqs []*ChangeRequest) error {
	return this.handler.ExecuteRequests(logger, zone, reqs)
}
