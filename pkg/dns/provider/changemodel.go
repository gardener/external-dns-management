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
	"net"
	"sort"
	"strings"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

////////////////////////////////////////////////////////////////////////////////
// Requests
////////////////////////////////////////////////////////////////////////////////

const (
	R_CREATE = "create"
	R_UPDATE = "update"
	R_DELETE = "delete"
)

type ChangeRequests []*ChangeRequest

type ChangeRequest struct {
	Action   string
	Type     string
	Addition *dns.DNSSet
	Deletion *dns.DNSSet
	Done     DoneHandler
}

func NewChangeRequest(action string, rtype string, del, add *dns.DNSSet, done DoneHandler) *ChangeRequest {
	return &ChangeRequest{Action: action, Type: rtype, Addition: add, Deletion: del, Done: done}
}

type ChangeGroup struct {
	name     string
	provider DNSProvider
	dnssets  dns.DNSSets
	requests ChangeRequests
}

func newChangeGroup(name string, provider DNSProvider) *ChangeGroup {
	return &ChangeGroup{name: name, provider: provider, dnssets: dns.DNSSets{}, requests: ChangeRequests{}}
}

func (this *ChangeGroup) cleanup(logger logger.LogContext, model *ChangeModel) bool {
	mod := false
	for _, s := range this.dnssets {
		_, ok := model.applied[s.Name]
		if !ok {
			if s.IsOwnedBy(model.owners) {
				model.Infof("found unapplied managed set '%s'", s.Name)
				for ty := range s.Sets {
					mod = true
					this.addDeleteRequest(s, ty, nil)
				}
			}
		}
	}
	return mod
}

func (this *ChangeGroup) update(logger logger.LogContext, model *ChangeModel) bool {
	ok := true
	model.Infof("reconcile entries for %s (with %d requests)", this.name, len(this.requests))

	reqs := this.requests
	if reqs != nil && len(reqs) > 0 {
		err := this.provider.ExecuteRequests(logger, model.zoneid, reqs)
		if err != nil {
			model.Errorf("entry reconcilation failed for %s: %s", this.name, err)
			ok = false
		}
	}
	return ok
}

func (this *ChangeGroup) addCreateRequest(dnsset *dns.DNSSet, rtype string, done DoneHandler) {
	this.addChangeRequest(R_CREATE, nil, dnsset, rtype, done)
}
func (this *ChangeGroup) addUpdateRequest(old, new *dns.DNSSet, rtype string, done DoneHandler) {
	this.addChangeRequest(R_UPDATE, old, new, rtype, done)
}
func (this *ChangeGroup) addDeleteRequest(dnsset *dns.DNSSet, rtype string, done DoneHandler) {
	this.addChangeRequest(R_DELETE, dnsset, nil, rtype, done)
}
func (this *ChangeGroup) addChangeRequest(action string, old, new *dns.DNSSet, rtype string, done DoneHandler) {
	r := NewChangeRequest(action, rtype, old, new, done)
	this.requests = append(this.requests, r)
}

////////////////////////////////////////////////////////////////////////////////
// Change Model
////////////////////////////////////////////////////////////////////////////////

type ChangeModel struct {
	logger.LogContext
	config         Config
	owners         utils.StringSet
	zoneid         string
	providers      DNSProviders
	applied        map[string]*dns.DNSSet
	dangling       *ChangeGroup
	providergroups map[DNSProvider]*ChangeGroup
}

func NewChangeModel(logger logger.LogContext, owners utils.StringSet, config Config, zoneid string, providers DNSProviders) *ChangeModel {
	return &ChangeModel{
		LogContext:     logger,
		config:         config,
		owners:         owners,
		zoneid:         zoneid,
		providers:      providers,
		applied:        map[string]*dns.DNSSet{},
		providergroups: map[DNSProvider]*ChangeGroup{},
	}
}

func (this *ChangeModel) getProviderView(p DNSProvider) *ChangeGroup {
	v := this.providergroups[p]
	if v == nil {
		v = newChangeGroup(p.ObjectName().String(), p)
		this.providergroups[p] = v
	}
	return v
}

func (this *ChangeModel) getDefaultProvider() DNSProvider {
	var provider DNSProvider
	for _, provider = range this.providers {
		break
	}
	return provider
}

func (this *ChangeModel) dumpf(fmt string, args ...interface{}) {
	this.Debugf(fmt, args...)
}

func (this *ChangeModel) Setup() error {
	provider := this.getDefaultProvider()
	if provider == nil {
		return fmt.Errorf("no provider found for zone %q", this.zoneid)
	}
	sets, err := provider.GetDNSSets(this.zoneid)
	if err != nil {
		return err
	}
	this.dangling = newChangeGroup("dangling entries", provider)
	for dnsName, set := range sets {
		var view *ChangeGroup
		provider = this.providers.LookupFor(dnsName)
		if provider != nil {
			this.dumpf("  %s: %d types (provider %s)", dnsName, len(set.Sets), provider.ObjectName())
			view = this.getProviderView(provider)
		} else {
			this.dumpf("  %s: %d types (no provider)", dnsName, len(set.Sets))
			view = this.dangling
		}
		view.dnssets[dnsName] = set
		for t, r := range set.Sets {
			this.dumpf("    %s: %d records: %s", t, len(r.Records), r.RecordString())
		}
	}
	this.Infof("found %d entries in zone %s (using %d groups)", len(sets), this.zoneid, len(this.providergroups))
	return err
}

func (this *ChangeModel) Check(name string, done DoneHandler, targets ...Target) (bool, error) {
	return this.Exec(false, name, done, targets...)
}
func (this *ChangeModel) Apply(name string, done DoneHandler, targets ...Target) (bool, error) {
	return this.Exec(true, name, done, targets...)
}
func (this *ChangeModel) Exec(apply bool, name string, done DoneHandler, targets ...Target) (bool, error) {
	if len(targets) == 0 {
		return false, nil
	}

	if apply {
		this.applied[name] = nil
	}
	p := this.providers.LookupFor(name)
	if p == nil {
		err := fmt.Errorf("no provider found for %q", name)
		if done != nil {
			done.SetInvalid(err)
		}
		return false, err
	}

	view := this.getProviderView(p)
	oldset := view.dnssets[name]
	newset := this.NewDNSSetForTargets(name, oldset, this.config.TTL, targets...)
	mod := false
	if oldset != nil {
		if this.IsForeign(oldset) {
			err := fmt.Errorf("dns name %q already busy for %q", name, oldset.GetOwner())
			if done != nil {
				done.SetInvalid(err)
			}
			return false, err
		} else {
			if !this.Owns(oldset) {
				this.Infof("catch entry %q by reassigning owner", name)
			}
			for ty, rset := range newset.Sets {
				curset := oldset.Sets[ty]
				if curset == nil {
					if apply {
						view.addCreateRequest(newset, ty, done)
					}
					mod = true
				} else {
					olddns, _ := dns.MapToProvider(ty, oldset)
					newdns, _ := dns.MapToProvider(ty, newset)
					if olddns == newdns {
						if !curset.Match(rset) {
							if apply {
								view.addUpdateRequest(oldset, newset, ty, done)
							}
							mod = true
						} else {
							if apply {
								this.Debugf("records type %s up to date for %s", ty, name)
							}
						}
					} else {
						mod = true
						if apply {
							view.addCreateRequest(newset, ty, done)
							view.addDeleteRequest(oldset, ty, nil)
						}
					}
				}
			}
			for ty := range oldset.Sets {
				if _, ok := newset.Sets[ty]; !ok {
					if apply {
						view.addDeleteRequest(oldset, ty, nil)
					}
					mod = true
				}
			}
		}
	} else {
		this.Infof("no existing entry found for %s", name)
		if apply {
			if this.config.Ident != "" {
				newset.SetOwner(this.config.Ident)
			}
			for ty := range newset.Sets {
				view.addCreateRequest(newset, ty, done)
			}
		}
		mod = true
	}
	if apply {
		this.applied[name] = newset
		if !mod && done != nil {
			done.Succeeded()
		}
	}
	return mod, nil
}

func (this *ChangeModel) Cleanup(logger logger.LogContext) bool {
	mod := false
	for _, view := range this.providergroups {
		mod = mod || view.cleanup(logger, this)
	}
	mod = mod || this.dangling.cleanup(logger, this)
	if mod {
		logger.Infof("found entries to be deleted")
	}
	return mod
}

func (this *ChangeModel) Update(logger logger.LogContext) error {
	failed := false
	for _, view := range this.providergroups {
		failed = failed || !view.update(logger, this)
	}
	failed = failed || !this.dangling.update(logger, this)
	if failed {
		return fmt.Errorf("entry reconcilation failed for some provider(s)")
	}
	return nil
}

/////////////////////////////////////////////////////////////////////////////////
// DNSSets

func (this *ChangeModel) Owns(set *dns.DNSSet) bool {
	return set.IsOwnedBy(this.owners)
}

func (this *ChangeModel) IsForeign(set *dns.DNSSet) bool {
	return set.IsForeign(this.owners)
}

func (this *ChangeModel) NewDNSSetForTargets(name string, base *dns.DNSSet, ttl int64, targets ...Target) *dns.DNSSet {
	set := dns.NewDNSSet(name)
	//if base != nil {
	//	meta := base.Sets[RS_META]
	//	if meta != nil {
	//		set.Sets[RS_META] = meta.Clone()
	//	}
	//}

	if base == nil || !this.IsForeign(base) {
		set.SetOwner(this.config.Ident)
		set.SetAttr(dns.ATTR_PREFIX, dns.TxtPrefix)
	}

	targetsets := set.Sets
	cnames := []string{}
	for _, t := range targets {
		ty := t.GetRecordType()
		if ty == dns.RS_CNAME && len(targets) > 1 {
			cnames = append(cnames, t.GetHostName())
			addrs, err := net.LookupHost(t.GetHostName())
			if err == nil {
				for _, addr := range addrs {
					AddRecord(targetsets, dns.RS_A, addr, ttl)
				}
			} else {
				this.Errorf("cannot lookup '%s': %s", t.GetHostName(), err)
			}
			this.Debugf("mapping target '%s' to A records: %s", t.GetHostName(), strings.Join(addrs, ","))
		} else {
			AddRecord(targetsets, ty, t.GetHostName(), ttl)
		}
	}
	set.Sets = targetsets
	if len(cnames) > 0 && this.Owns(set) {
		sort.Strings(cnames)
		set.SetAttr(dns.ATTR_CNAMES, strings.Join(cnames, ","))
	}
	return set
}

func AddRecord(targetsets dns.RecordSets, ty string, host string, ttl int64) {
	rs := targetsets[ty]
	if rs == nil {
		rs = dns.NewRecordSet(ty, ttl, nil)
		targetsets[ty] = rs
	}
	rs.Records = append(rs.Records, &dns.Record{host})
}
