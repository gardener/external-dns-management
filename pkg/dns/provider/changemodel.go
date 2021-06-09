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
	"sort"
	"strings"
	"time"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"

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
	model    *ChangeModel
}

func newChangeGroup(name string, provider DNSProvider, model *ChangeModel) *ChangeGroup {
	return &ChangeGroup{name: name, provider: provider, dnssets: dns.DNSSets{}, requests: ChangeRequests{}, model: model}
}

func (this *ChangeGroup) cleanup(logger logger.LogContext, model *ChangeModel) bool {
	mod := false
	for _, s := range this.dnssets {
		_, ok := model.applied[s.Name]
		if !ok {
			if s.IsOwnedBy(model.owners) {
				if e := model.IsStale(s.Name); e != nil {
					if e.IsDeleting() {
						model.failedDNSNames.Add(s.Name) // preventing deletion of stale entry
					}
					status := e.Object().BaseStatus()
					msg := MSG_PRESERVED
					trigger := false
					if status.State == api.STATE_ERROR || status.State == api.STATE_INVALID {
						msg = msg + ": " + utils.StringValue(status.Message)
						model.Infof("found stale set '%s': %s -> preserve unchanged", utils.StringValue(status.Message), s.Name)
					} else {
						model.Infof("found stale set '%s' -> preserve unchanged", s.Name)
						trigger = true
					}
					upd, err := e.UpdateStatus(logger, api.STATE_STALE, msg)
					if trigger && (!upd || err != nil) {
						e.Trigger(logger)
					}
				} else {
					model.Infof("found unapplied managed set '%s'", s.Name)
					var done DoneHandler
					for _, e := range model.context.entries {
						if e.dnsname == s.Name {
							done = NewStatusUpdate(logger, e, model.context.fhandler)
							break
						}
					}
					for ty := range s.Sets {
						mod = true
						this.addDeleteRequest(s, ty, model.wrappedDoneHandler(s.Name, done))
					}
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
	if len(reqs) > 0 {
		err := this.provider.ExecuteRequests(logger, model.context.zone.getZone(), this.model.zonestate, reqs)
		if err != nil {
			model.Errorf("entry reconciliation failed for %s: %s", this.name, err)
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
	context        *zoneReconciliation
	applied        map[string]*dns.DNSSet
	dangling       *ChangeGroup
	providergroups map[string]*ChangeGroup
	zonestate      DNSZoneState
	failedDNSNames utils.StringSet
}

type ChangeResult struct {
	Modified bool
	Retry    bool
	Error    error
}

func NewChangeModel(logger logger.LogContext, owners utils.StringSet, req *zoneReconciliation, config Config) *ChangeModel {
	return &ChangeModel{
		LogContext:     logger,
		config:         config,
		owners:         owners,
		context:        req,
		applied:        map[string]*dns.DNSSet{},
		providergroups: map[string]*ChangeGroup{},
		failedDNSNames: utils.StringSet{},
	}
}

func (this *ChangeModel) IsStale(dns string) *Entry {
	return this.context.stale[dns]
}

func (this *ChangeModel) getProviderView(p DNSProvider) *ChangeGroup {
	v := this.providergroups[p.AccountHash()]
	if v == nil {
		v = newChangeGroup(p.ObjectName().String(), p, this)
		this.providergroups[p.AccountHash()] = v
	}
	return v
}

func (this *ChangeModel) ZoneId() string {
	return this.context.zone.Id()
}

func (this *ChangeModel) Domain() string {
	return this.context.zone.Domain()
}

func (this *ChangeModel) getDefaultProvider() DNSProvider {
	var provider DNSProvider
	for _, provider = range this.context.providers {
		break
	}
	return provider
}

func (this *ChangeModel) dumpf(fmt string, args ...interface{}) {
	this.Debugf(fmt, args...)
}

func (this *ChangeModel) Setup() error {
	var err error

	provider := this.getDefaultProvider()
	if provider == nil {
		return fmt.Errorf("no provider found for zone %q", this.ZoneId())
	}
	this.zonestate, err = provider.GetZoneState(this.context.zone.getZone())
	if err != nil {
		return err
	}
	sets := this.zonestate.GetDNSSets()
	this.context.zone.SetOwners(sets.GetOwners())
	this.dangling = newChangeGroup("dangling entries", provider, this)
	for dnsName, set := range sets {
		var view *ChangeGroup
		provider = this.context.providers.LookupFor(dnsName)
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
	this.Infof("found %d entries in zone %s (using %d groups)", len(sets), this.ZoneId(), len(this.providergroups))
	return err
}

/*
func (this *ChangeModel) Check(name string, createdAt time.Time, done DoneHandler, targets ...Target) ChangeResult {
	return this.Exec(false, false, name, createdAt, done, targets...)
}
*/
func (this *ChangeModel) Apply(name string, createdAt time.Time, done DoneHandler, kind string, targets ...Target) ChangeResult {
	return this.Exec(true, false, name, createdAt, done, kind, targets...)
}
func (this *ChangeModel) Delete(name string, createdAt time.Time, done DoneHandler, kind string) ChangeResult {
	return this.Exec(true, true, name, createdAt, done, kind)
}

func (this *ChangeModel) Exec(apply bool, delete bool, name string, createdAt time.Time, done DoneHandler, kind string, targets ...Target) ChangeResult {
	//this.Infof("%s: %v", name, targets)
	if len(targets) == 0 && !delete {
		return ChangeResult{}
	}

	if apply {
		this.applied[name] = nil
		done = this.wrappedDoneHandler(name, done)
	}
	p := this.context.providers.LookupFor(name)
	if p == nil {
		err := fmt.Errorf("no provider found for %q", name)
		if done != nil {
			if apply {
				done.SetInvalid(err)
			}
		} else {
			this.Warnf("no done handler and %s", err)
		}
		return ChangeResult{Error: err}
	}

	view := this.getProviderView(p)
	oldset := view.dnssets[name]
	newset := dns.NewDNSSet(name)
	newset.SetKind(kind)
	if !delete {
		this.AddTargets(newset, oldset, p, targets...)
	}
	mod := false
	if oldset != nil {
		if this.IsForeign(oldset, newset) {
			err := &perrs.AlreadyBusyForOwner{DNSName: name, EntryCreatedAt: createdAt, Owner: oldset.GetOwner()}
			retry := p.ReportZoneStateConflict(this.context.zone.getZone(), err)
			if done != nil {
				if apply && !retry {
					done.SetInvalid(err)
				}
			} else {
				this.Warnf("no done handler and %s", err)
			}
			return ChangeResult{Error: err, Retry: retry}
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
					olddns, _ := dns.MapToProvider(ty, oldset, this.Domain())
					newdns, _ := dns.MapToProvider(ty, newset, this.Domain())
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
						if apply {
							view.addCreateRequest(newset, ty, done)
							view.addDeleteRequest(oldset, ty, this.wrappedDoneHandler(name, nil))
						}
						mod = true
					}
				}
			}
			for ty := range oldset.Sets {
				if _, ok := newset.Sets[ty]; !ok {
					if apply {
						view.addDeleteRequest(oldset, ty, done)
					}
					mod = true
				}
			}
		}
	} else {
		if !delete {
			this.Debugf("no existing entry found for %s", name)
			if apply {
				this.setOwner(newset, targets)
				for ty := range newset.Sets {
					view.addCreateRequest(newset, ty, done)
				}
			}
			mod = true
		}
	}
	if apply {
		this.applied[name] = newset
		if !mod && done != nil {
			done.Succeeded()
		}
	}
	return ChangeResult{Modified: mod}
}

func (this *ChangeModel) Cleanup(logger logger.LogContext) bool {
	mod := false
	for _, view := range this.providergroups {
		mod = view.cleanup(logger, this) || mod
	}
	mod = this.dangling.cleanup(logger, this) || mod
	if mod {
		logger.Infof("found entries to be deleted")
	}
	return mod
}

func (this *ChangeModel) Update(logger logger.LogContext) error {
	failed := false
	for _, view := range this.providergroups {
		failed = !view.update(logger, this) || failed
	}
	failed = !this.dangling.update(logger, this) || failed
	if failed {
		return fmt.Errorf("entry reconciliation failed for some provider(s)")
	}
	return nil
}

func (this *ChangeModel) IsFailed(dnsName string) bool {
	return this.failedDNSNames.Contains(dnsName)
}

func (this *ChangeModel) wrappedDoneHandler(dnsName string, done DoneHandler) DoneHandler {
	return &changeModelDoneHandler{
		changeModel: this,
		inner:       done,
		dnsName:     dnsName,
	}
}

/////////////////////////////////////////////////////////////////////////////////
// changeModelDoneHandler

type changeModelDoneHandler struct {
	changeModel *ChangeModel
	inner       DoneHandler
	dnsName     string
}

func (this *changeModelDoneHandler) SetInvalid(err error) {
	if this.inner != nil {
		this.inner.SetInvalid(err)
	}
}

func (this *changeModelDoneHandler) Failed(err error) {
	this.changeModel.failedDNSNames.Add(this.dnsName)
	if this.inner != nil {
		this.inner.Failed(err)
	}
}

func (this *changeModelDoneHandler) Succeeded() {
	if this.inner != nil {
		this.inner.Succeeded()
	}
}

/////////////////////////////////////////////////////////////////////////////////
// DNSSets

func (this *ChangeModel) Owns(set *dns.DNSSet) bool {
	return set.GetKind() != api.DNSLockKind && set.IsOwnedBy(this.owners)
}

func (this *ChangeModel) IsForeign(set *dns.DNSSet, refset *dns.DNSSet) bool {
	if set.GetKind() == api.DNSLockKind {
		return set.GetOwner() != refset.GetOwner()
	}
	return set.IsForeign(this.owners)
}

func (this *ChangeModel) setOwner(set *dns.DNSSet, targets []Target) bool {
	id := targets[0].GetEntry().OwnerId()
	if id == "" {
		id = this.config.Ident
	}
	if id != "" {
		set.SetOwner(id)
		return true
	}
	return false
}

func (this *ChangeModel) AddTargets(set *dns.DNSSet, base *dns.DNSSet, provider DNSProvider, targets ...Target) *dns.DNSSet {
	if base == nil || !this.IsForeign(base, set) {
		if this.setOwner(set, targets) {
			set.SetMetaAttr(dns.ATTR_PREFIX, dns.TxtPrefix)
		}
	}

	targetsets := set.Sets
	cnames := []string{}
	for _, t := range targets {
		// use status calculated in entry
		ttl := t.GetEntry().TTL()
		if t.GetRecordType() == dns.RS_CNAME && len(targets) > 1 {
			cnames = append(cnames, t.GetHostName())
			addrs, err := lookupHostIPv4(t.GetHostName())
			if err == nil {
				for _, addr := range addrs {
					AddRecord(targetsets, dns.RS_A, addr, ttl)
				}
			} else {
				this.Errorf("cannot lookup '%s': %s", t.GetHostName(), err)
			}
			this.Debugf("mapping target '%s' to A records: %s", t.GetHostName(), strings.Join(addrs, ","))
		} else {
			t = provider.MapTarget(t)
			AddRecord(targetsets, t.GetRecordType(), t.GetHostName(), ttl)
		}
	}
	set.Sets = targetsets
	if len(cnames) > 0 && this.Owns(set) {
		sort.Strings(cnames)
		set.SetMetaAttr(dns.ATTR_CNAMES, strings.Join(cnames, ","))
	}
	return set
}

func AddRecord(targetsets dns.RecordSets, ty string, host string, ttl int64) {
	rs := targetsets[ty]
	if rs == nil {
		rs = dns.NewRecordSet(ty, ttl, nil)
		targetsets[ty] = rs
	}
	rs.Records = append(rs.Records, &dns.Record{Value: host})
}
