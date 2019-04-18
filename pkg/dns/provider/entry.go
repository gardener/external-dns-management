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
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gardener/external-dns-management/pkg/dns"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	corev1 "k8s.io/api/core/v1"
)

const MSG_PRESERVED = "errornous entry preserved in provider"

type EntryPremise struct {
	ptypes   utils.StringSet
	ptype    string
	provider DNSProvider
	zoneid   string
}

func (this *EntryPremise) Equals(p *EntryPremise) bool {
	return this.ptype == p.ptype && this.provider == p.provider && this.zoneid == p.zoneid
}

func (this *EntryPremise) NotifyChange(p *EntryPremise) string {
	r := []string{}
	if this.ptype != p.ptype {
		r = append(r, fmt.Sprintf("provider type (%s -> %s)", this.ptype, p.ptype))
	}
	if this.provider != p.provider {
		r = append(r, fmt.Sprintf("provider (%s -> %s)", Provider(this.provider), Provider(p.provider)))
	}
	if this.zoneid != p.zoneid {
		r = append(r, fmt.Sprintf("provider (%s -> %s)", this.zoneid, p.zoneid))
	}
	if len(r) == 0 {
		return ""
	}
	return "premise changed: " + strings.Join(r, ", ")
}

type EntryVersion struct {
	object   *dnsutils.DNSEntryObject
	dnsname  string
	targets  Targets
	mappings map[string][]string
	warnings []string

	status api.DNSEntryStatus

	interval  int64
	valid     bool
	duplicate bool
}

func NewEntryVersion(object *dnsutils.DNSEntryObject, old *Entry) *EntryVersion {
	v := &EntryVersion{
		object:   object,
		dnsname:  object.DNSEntry().Spec.DNSName,
		targets:  Targets{},
		mappings: map[string][]string{},
	}
	if old != nil {
		v.status = old.status
	} else {
		v.status = *object.Status()
	}
	return v
}

func (this *EntryVersion) RequiresUpdateFor(e *EntryVersion) (reasons []string) {
	if this.dnsname != e.dnsname {
		reasons = append(reasons, "dnsname changed")
	}
	if !utils.Int64Equal(this.status.TTL, e.status.TTL) {
		reasons = append(reasons, "ttl changed")
	}
	if this.valid != e.valid {
		reasons = append(reasons, "validation state changed")
	}
	if this.ZoneId() != e.ZoneId() {
		reasons = append(reasons, "zone changed")
	}
	if this.targets.DifferFrom(e.targets) {
		reasons = append(reasons, "targets changed")
	}
	if this.State() != e.State() {
		if e.State() != api.STATE_READY {
			reasons = append(reasons, "state changed")
		}
	}
	return
}

func (this *EntryVersion) IsValid() bool {
	return this.valid
}

func (this *EntryVersion) IsDeleting() bool {
	return this.object.IsDeleting()
}

func (this *EntryVersion) Object() *dnsutils.DNSEntryObject {
	return this.object
}

func (this *EntryVersion) Message() string {
	return utils.StringValue(this.status.Message)
}

func (this *EntryVersion) ZoneId() string {
	return utils.StringValue(this.status.Zone)
}

func (this *EntryVersion) State() string {
	return this.status.State
}

func (this *EntryVersion) ClusterKey() resources.ClusterObjectKey {
	return this.object.ClusterKey()
}

func (this *EntryVersion) ObjectName() resources.ObjectName {
	return this.object.ObjectName()
}

func (this *EntryVersion) DNSName() string {
	return this.dnsname
}

func (this *EntryVersion) Targets() Targets {
	return this.targets
}

func (this *EntryVersion) Description() string {
	return this.object.Description()
}

func (this *EntryVersion) TTL() int64 {
	return utils.Int64Value(this.status.TTL, 0)
}

func (this *EntryVersion) Interval() int64 {
	return this.interval
}

func (this *EntryVersion) OwnerId() string {
	if this.object.GetOwnerId() != nil {
		return *this.object.GetOwnerId()
	}
	return ""
}

func validate(state *state, entry *EntryVersion) (targets Targets, warnings []string, err error) {
	spec := &entry.object.DNSEntry().Spec

	targets = Targets{}
	warnings = []string{}

	check := entry.object.GetDNSName()
	if strings.HasPrefix(check, "*.") {
		check = check[2:]
	}
	if errs := validation.IsDNS1123Subdomain(check); errs != nil {
		if werrs := validation.IsWildcardDNS1123Subdomain(check); werrs != nil {
			err = fmt.Errorf("%q is no valid dns name (%v)", check, append(errs, werrs...))
			return
		}
	}
	if len(spec.Targets) > 0 && len(spec.Text) > 0 {
		err = fmt.Errorf("only Text or Targets possible: %s", err)
		return
	}
	if spec.TTL != nil && (*spec.TTL == 0 || *spec.TTL < 0) {
		err = fmt.Errorf("TTL must be  greater than zero: %s", err)
		return
	}

	for _, t := range spec.Targets {
		new := NewTargetFromEntryVersion(t, entry)
		if targets.Has(new) {
			warnings = append(warnings, fmt.Sprintf("dns entry %q has duplicate target %q", entry.ObjectName(), new))
		} else {
			targets = append(targets, new)
		}
	}
	for _, t := range spec.Text {
		new := NewText(t, entry)
		if targets.Has(new) {
			warnings = append(warnings, fmt.Sprintf("dns entry %q has duplicate text %q", entry.ObjectName(), new))
		} else {
			targets = append(targets, new)
		}
	}

	if utils.StringValue(spec.OwnerId) != "" {
		if !state.ownerCache.IsResponsibleFor(*spec.OwnerId) {
			err = fmt.Errorf("unknown owner id '%s'", *spec.OwnerId)
		}
	}
	if len(targets) == 0 {
		err = fmt.Errorf("no target or text specified")
	}
	return
}

func (this *EntryVersion) Setup(logger logger.LogContext, state *state, p *EntryPremise, op string, err error, defaultTTL int64, old *Entry) reconcile.Status {

	hello := dnsutils.NewLogMessage("%s ENTRY: %s, zoneid: %s, handler: %s, provider: %s", op, this.Object().Status().State, p.zoneid, p.ptype, Provider(p.provider))

	hello.Infof(logger)
	this.valid = false
	spec := &this.object.DNSEntry().Spec

	///////////// handle type responsibility

	if !utils.IsEmptyString(this.object.Status().ProviderType) && p.ptype == "" {
		// other controller claimed responsibility?
		this.status.ProviderType = this.object.Status().ProviderType
	}

	if utils.IsEmptyString(this.status.ProviderType) || (p.zoneid != "" && *this.status.ProviderType != p.ptype) {
		if p.zoneid == "" {
			// mark unassigned foreign entries as errorneous
			if this.object.GetCreationTimestamp().Add(120 * time.Second).After(time.Now()) {
				state.RemoveFinalizer(this.object)
				return reconcile.Succeeded(logger).RescheduleAfter(120 * time.Second)
			}
			hello.Infof(logger)
			logger.Infof("probably no responsible controller found -> mark as error")
			this.status.Provider = nil
			this.status.ProviderType = nil
			this.status.Zone = nil
			err := this.updateStatus(logger, api.STATE_ERROR, "No responsible provider found")
			if err != nil {
				return reconcile.Delay(logger, err)
			}
		} else {
			// assign entry to actual type
			hello.Infof(logger)
			logger.Infof("assigning to provider type %q responsible for zone %s", p.ptype, p.zoneid)
			this.status.State = api.STATE_PENDING
			this.status.Message = StatusMessage("waiting for dns reconcilation")
		}
	}

	if p.zoneid == "" && !utils.IsEmptyString(this.status.ProviderType) && p.ptypes.Contains(*this.status.ProviderType) {
		// revoke assignment to actual type
		hello.Infof(logger)
		old := utils.StringValue(this.status.ProviderType)
		this.status.Provider = nil
		this.status.ProviderType = nil
		this.status.Zone = nil
		err := this.updateStatus(logger, "", "not valid for known provider anymore -> releasing provider type %s", old)
		if err != nil {
			return reconcile.Delay(logger, err)
		}
	}

	if p.zoneid == "" || p.ptype == "" {
		return reconcile.RepeatOnError(logger, state.RemoveFinalizer(this.object))
	}

	provider := ""
	this.status.Zone = &p.zoneid
	this.status.ProviderType = &p.ptype
	if p.provider != nil {
		provider = p.provider.ObjectName().String()
		this.status.Provider = &provider
		this.status.TTL = &defaultTTL
		if spec.TTL != nil {
			this.status.TTL = spec.TTL
		}
	} else {
		this.status.Provider = nil
		this.status.TTL = nil
	}

	spec = &this.object.DNSEntry().Spec

	///////////// validate

	targets, warnings, verr := validate(state, this)

	if verr != nil {
		hello.Infof(logger, "validation failed: %s", verr)

		state := api.STATE_INVALID
		if this.status.State == api.STATE_READY {
			state = api.STATE_STALE
		}
		this.UpdateStatus(logger, state, verr.Error())
		return reconcile.Failed(logger, verr)
	}

	///////////// handle

	hello.Infof(logger, "validation ok")

	if this.IsDeleting() {
		logger.Infof("update state to %s", api.STATE_DELETING)
		this.status.State = api.STATE_DELETING
		this.status.Message = StatusMessage("entry is scheduled to be deleted")
		this.valid = true
	} else {
		this.warnings = warnings
		targets, mappings := normalizeTargets(logger, this.object, targets...)
		if len(mappings) > 0 {
			if spec.CNameLookupInterval != nil && *spec.CNameLookupInterval > 0 {
				this.interval = *spec.CNameLookupInterval
			} else {
				this.interval = 600
			}
		} else {
			this.interval = 0
		}

		this.targets = targets
		if err != nil {
			if this.status.State != api.STATE_STALE {
				this.status.State = api.STATE_ERROR
				this.status.Message = StatusMessage(err.Error())
			} else {
				if strings.HasPrefix(*this.status.Message, MSG_PRESERVED) {
					this.status.Message = StatusMessage(MSG_PRESERVED + ": " + err.Error())
				} else {
					this.status.Message = StatusMessage(err.Error())
				}
			}
		} else {
			if p.zoneid == "" {
				this.status.State = api.STATE_ERROR
				this.status.Provider = nil
				this.status.Message = StatusMessagef("no provider found for %q", this.dnsname)
			} else {
				this.valid = true
			}
		}
	}

	logger.Infof("%s: valid: %t, message: %s, err: %s", this.status.State, this.valid, utils.StringValue(this.status.Message), ErrorValue(err))
	logmsg := dnsutils.NewLogMessage("update entry status")
	f := func(data resources.ObjectData) (bool, error) {
		e := data.(*api.DNSEntry)
		mod := &utils.ModificationState{}
		if p.zoneid != "" {
			mod.AssureStringPtrValue(&e.Status.ProviderType, p.ptype)
		}
		mod.AssureStringValue(&e.Status.State, this.status.State).
			AssureStringPtrPtr(&e.Status.Message, this.status.Message).
			AssureStringPtrPtr(&e.Status.Zone, this.status.Zone).
			AssureStringPtrPtr(&e.Status.Provider, this.status.Provider)
		if mod.IsModified() {
			logmsg.Infof(logger)
		}
		return mod.IsModified(), nil
	}
	_, err = this.object.ModifyStatus(f)
	return reconcile.DelayOnError(logger, err)
}

func (this *EntryVersion) updateStatus(logger logger.LogContext, state, msg string, args ...interface{}) error {
	logmsg := dnsutils.NewLogMessage(msg, args...)
	f := func(data resources.ObjectData) (bool, error) {
		e := data.(*api.DNSEntry)
		mod := (&utils.ModificationState{}).
			AssureStringPtrPtr(&e.Status.ProviderType, this.status.ProviderType).
			AssureStringValue(&e.Status.State, state).
			AssureStringPtrValue(&e.Status.Message, logmsg.Get()).
			AssureStringPtrPtr(&e.Status.Zone, this.status.Zone).
			AssureStringPtrPtr(&e.Status.Provider, this.status.Provider).
			AssureInt64PtrPtr(&e.Status.TTL, this.status.TTL)
		if state != "" && e.Status.ObservedGeneration < this.object.GetGeneration() {
			mod.AssureInt64Value(&e.Status.ObservedGeneration, this.object.GetGeneration())
		}
		if utils.StringValue(this.status.Provider) == "" {
			if e.Status.Targets != nil {
				e.Status.Targets = nil
				mod.Modify(true)
			}
		}
		if mod.IsModified() {
			logmsg.Infof(logger)
		}
		return mod.IsModified(), nil
	}
	_, err := this.object.ModifyStatus(f)
	this.object.Event(corev1.EventTypeNormal, "reconcile", logmsg.Get())
	return err
}

func (this *EntryVersion) UpdateStatus(logger logger.LogContext, state string, msg string) (bool, error) {
	f := func(data resources.ObjectData) (bool, error) {
		o := data.(*api.DNSEntry)
		if state == api.STATE_PENDING && o.Status.State != "" {
			return false, nil
		}
		mod := &utils.ModificationState{}

		if state == api.STATE_READY {
			mod.AssureInt64PtrPtr(&o.Status.TTL, this.status.TTL)
			list, msg := targetList(this.targets)
			if !reflect.DeepEqual(list, o.Status.Targets) {
				o.Status.Targets = list
				logger.Info(msg)
				mod.Modify(true)
			}
			if this.status.Provider != nil {
				mod.AssureStringPtrPtr(&o.Status.Provider, this.status.Provider)
			}
		}
		mod.AssureInt64Value(&o.Status.ObservedGeneration, o.Generation)
		if !(this.status.State == api.STATE_STALE && this.status.State == state) {
			mod.AssureStringPtrValue(&o.Status.Message, msg)
			this.status.Message = &msg
		}
		if !(this.status.State == api.STATE_STALE && state == api.STATE_INVALID) {
			mod.AssureStringValue(&o.Status.State, state)
			this.status.State = state
		}
		if mod.IsModified() {
			logger.Infof("update state of '%s/%s' to %s (%s)", o.Namespace, o.Name, state, msg)
		}
		return mod.IsModified(), nil
	}
	return this.object.ModifyStatus(f)
}

func targetList(targets Targets) ([]string, string) {
	list := []string{}
	msg := "update effective targets: ["
	sep := ""
	for _, t := range targets {
		list = append(list, t.GetHostName())
		msg = fmt.Sprintf("%s%s%s", msg, sep, t)
		sep = ", "
	}
	msg = msg + "]"
	return list, msg
}

func normalizeTargets(logger logger.LogContext, object *dnsutils.DNSEntryObject, targets ...Target) (Targets, map[string][]string) {
	result := make(Targets, 0, len(targets))
	mappings := map[string][]string{}
	for _, t := range targets {
		ty := t.GetRecordType()
		if ty == dns.RS_CNAME && len(targets) > 1 {
			addrs, err := net.LookupHost(t.GetHostName())
			if err == nil {
				for _, addr := range addrs {
					result = append(result, NewTarget(dns.RS_A, addr, t.GetEntry()))
				}
			} else {
				w := fmt.Sprintf("cannot lookup '%s': %s", t.GetHostName(), err)
				logger.Warn(w)
				object.Event(corev1.EventTypeNormal, "dnslookup", w)
			}
			mappings[t.GetHostName()] = addrs
		} else {
			result = append(result, t)
		}
	}
	return result, mappings
}

///////////////////////////////////////////////////////////////////////////////

type Entry struct {
	lock       sync.Mutex
	key        string
	modified   bool
	activezone string
	state      *state

	*EntryVersion
}

func NewEntry(v *EntryVersion, state *state) *Entry {
	return &Entry{
		key:          v.ObjectName().String(),
		EntryVersion: v,
		state:        state,
		modified:     true,
		activezone:   utils.StringValue(v.status.Zone),
	}
}

func (this *Entry) Trigger(logger logger.LogContext) {
	this.state.TriggerEntry(logger, this)
}

func (this *Entry) IsActive() bool {
	id := this.OwnerId()
	if id == "" {
		id = this.state.config.Ident
	}
	return this.state.ownerCache.IsResponsibleFor(id)
}

func (this *Entry) IsModified() bool {
	return this.modified
}

func (this *Entry) Update(logger logger.LogContext, new *EntryVersion) *Entry {

	if this.DNSName() != new.DNSName() {
		return NewEntry(new, this.state)
	}

	reasons := this.RequiresUpdateFor(new)
	if len(reasons) != 0 {
		logger.Infof("update actual entry: valid: %t  %v", new.IsValid(), reasons)
		if this.targets.DifferFrom(new.targets) && !new.IsDeleting() {
			logger.Infof("targets differ from internal state")
			for _, w := range new.warnings {
				logger.Warn(w)
				this.object.Event(corev1.EventTypeNormal, "reconcile", w)
			}
			for dns, m := range new.mappings {
				msg := fmt.Sprintf("mapping cname %q to %v", dns, m)
				logger.Info(msg)
				this.object.Event(corev1.EventTypeNormal, "dnslookup", msg)
			}
			_, msg := targetList(new.targets)
			logger.Infof("%s", msg)
		}
		this.modified = true
	}
	this.EntryVersion = new

	return this
}

func (this *Entry) Before(e *Entry) bool {
	if e == nil {
		return true
	}
	if this.Object().GetCreationTimestamp().Time.Equal(e.Object().GetCreationTimestamp().Time) {
		// for entries created at same time compare objectname to define strict order
		return strings.Compare(this.ObjectName().String(), e.ObjectName().String()) < 0
	}
	return this.Object().GetCreationTimestamp().Time.Before(e.Object().GetCreationTimestamp().Time)
}

////////////////////////////////////////////////////////////////////////////////
// Entries
////////////////////////////////////////////////////////////////////////////////

type Entries map[resources.ObjectName]*Entry

func (this Entries) AddEntry(entry *Entry) *Entry {
	old := this[entry.ObjectName()]
	this[entry.ObjectName()] = entry
	if old != nil && old != entry {
		return old
	}
	return nil
}

func (this Entries) Delete(e *Entry) {
	if this[e.ObjectName()] == e {
		delete(this, e.ObjectName())
	}
}

func testUpdate(msg string, object resources.Object) {
	err := object.UpdateStatus()
	logger.Infof("**** %s %s %s: status update: %s", msg, object.ObjectName(), object.GetResourceVersion(), err)
	err = object.Update()
	logger.Infof("update: %s", err)
}

////////////////////////////////////////////////////////////////////////////////
// EntryList
////////////////////////////////////////////////////////////////////////////////

type EntryList []*Entry

func (this EntryList) Len() int {
	return len(this)
}

func (this EntryList) Less(i, j int) bool {
	return strings.Compare(this[i].key, this[j].key) < 0
}

func (this EntryList) Swap(i, j int) {
	this[i], this[j] = this[j], this[i]
}

func (this EntryList) Sort() {
	sort.Sort(this)
}

func (this EntryList) Lock() {
	this.Sort()
	for _, e := range this {
		e.lock.Lock()
	}
}

func (this EntryList) Unlock() {
	for _, e := range this {
		e.lock.Unlock()
	}
}

func StatusMessage(s string) *string {
	return &s
}
func StatusMessagef(msgfmt string, args ...interface{}) *string {
	return StatusMessage(fmt.Sprintf(msgfmt, args...))
}

func Provider(p DNSProvider) string {
	if p == nil {
		return "<none>"
	}
	return p.ObjectName().String()
}
