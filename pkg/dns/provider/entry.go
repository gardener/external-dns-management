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

type Entry struct {
	lock      sync.Mutex
	object    *dnsutils.DNSEntryObject
	dnsname   string
	targets   Targets
	mappings  map[string][]string
	ttl       int64
	interval  int64
	valid     bool
	modified  bool
	duplicate bool
}

func NewEntry(object *dnsutils.DNSEntryObject) *Entry {
	return &Entry{
		object:   object,
		dnsname:  object.DNSEntry().Spec.DNSName,
		targets:  Targets{},
		mappings: map[string][]string{},
	}
}

func (this *Entry) Object() resources.Object {
	return this.object.Object
}

func (this *Entry) ClusterKey() resources.ClusterObjectKey {
	return this.object.ClusterKey()
}

func (this *Entry) ObjectName() resources.ObjectName {
	return this.object.ObjectName()
}

func (this *Entry) DNSName() string {
	return this.dnsname
}

func (this *Entry) Description() string {
	return this.object.Description()
}

func (this *Entry) TTL() int64 {
	return this.ttl
}

func (this *Entry) Interval() int64 {
	return this.interval
}

func (this *Entry) OwnerId() string {
	if this.object.GetOwnerId() != nil {
		return *this.object.GetOwnerId()
	}
	return ""
}

func (this *Entry) Targets() Targets {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.targets
}

func (this *Entry) IsValid() bool {
	this.lock.Lock()
	this.lock.Unlock()
	return this.valid
}

func (this *Entry) IsDeleting() bool {
	this.lock.Lock()
	this.lock.Unlock()
	return this.object.IsDeleting()
}

func (this *Entry) IsModified() bool {
	this.lock.Lock()
	this.lock.Unlock()
	return this.modified
}

func (this *Entry) Validate(ownerids utils.StringSet) (targets Targets, warnings []string, err error) {

	spec := &this.object.DNSEntry().Spec

	targets = Targets{}
	warnings = []string{}

	if this.dnsname != spec.DNSName {
		panic(fmt.Sprintf("change the dnsname should be handled by replacing the entry object (%q)", this.ObjectName()))
	}

	check := this.dnsname
	if strings.HasPrefix(this.dnsname, "*.") {
		check = this.dnsname[2:]
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
		new := NewTargetFromEntry(t, this)
		if targets.Has(new) {
			warnings = append(warnings, fmt.Sprintf("dns entry %q has duplicate target %q", this.ObjectName(), new))
		} else {
			targets = append(targets, new)
		}
	}
	for _, t := range spec.Text {
		new := NewText(t, this)
		if targets.Has(new) {
			warnings = append(warnings, fmt.Sprintf("dns entry %q has duplicate text %q", this.ObjectName(), new))
		} else {
			targets = append(targets, new)
		}
	}

	if utils.StringValue(spec.OwnerId) != "" {
		if !ownerids.Contains(*spec.OwnerId) {
			err = fmt.Errorf("unknown owner id '%s'", *spec.OwnerId)
		}
	}
	if len(targets) == 0 {
		err = fmt.Errorf("no target or text specified")
	}
	return
}

func (this *Entry) Update(logger logger.LogContext, ownerids utils.StringSet, object *dnsutils.DNSEntryObject, resp, zoneid string, err error, defaultTTL int64) reconcile.Status {
	this.lock.Lock()
	this.lock.Unlock()

	this.object = object

	curvalid := this.valid
	this.valid = false
	status := &this.object.DNSEntry().Status

	///////////// handle type responsibility

	if utils.IsEmptyString(status.ProviderType) || (*status.ProviderType != resp && zoneid != "") {
		if zoneid == "" {
			// mark unassigned foreign entries as errorneous
			if object.GetCreationTimestamp().Add(120 * time.Second).After(time.Now()) {
				return reconcile.Succeeded(logger).RescheduleAfter(120 * time.Second)
			}
			f := func(data resources.ObjectData) (bool, error) {
				e := data.(*api.DNSEntry)
				if !utils.IsEmptyString(e.Status.ProviderType) {
					return false, nil
				}
				mod := utils.ModificationState{}
				mod.AssureStringValue(&e.Status.State, api.STATE_ERROR)
				mod.AssureStringPtrValue(&e.Status.Message, "No responsible provider found")
				logger.Errorf("no responsible provider found")
				return mod.IsModified(), nil
			}
			_, err := object.ModifyStatus(f)
			return reconcile.DelayOnError(logger, err)
		} else {
			// assign entry to actual type
			msg := fmt.Sprintf("Assigned to provider type %q responsible for zone %s", resp, zoneid)
			f := func(data resources.ObjectData) (bool, error) {
				e := data.(*api.DNSEntry)
				mod := (&utils.ModificationState{}).
					AssureStringPtrValue(&e.Status.ProviderType, resp).
					AssureStringValue(&e.Status.State, api.STATE_PENDING).
					AssureStringPtrValue(&e.Status.Message, msg).
					AssureStringPtrValue(&e.Status.Zone, zoneid)
				return mod.IsModified(), nil
			}
			_, err := object.ModifyStatus(f)
			if err != nil {
				return reconcile.Delay(logger, err)
			}
			this.object.Event(corev1.EventTypeNormal, "reconcile", msg)
		}
	}

	if utils.StringValue(status.ProviderType) != resp {
		return reconcile.Succeeded(logger)
	}

	///////////// validate

	targets, warnings, verr := this.Validate(ownerids)

	if verr != nil {
		state := api.STATE_INVALID
		if status.State == api.STATE_READY {
			state = api.STATE_STALE
		}
		this.UpdateStatus(logger, state, verr.Error(), nil)
		return reconcile.Failed(logger, verr)
	}

	///////////// handle

	mod := resources.NewModificationState(this.object)

	if object.IsDeleting() {
		logger.Infof("update state to %s", api.STATE_DELETING)
		mod.AssureStringValue(&status.State, api.STATE_DELETING)
		mod.AssureStringPtrValue(&status.Message, "entry is scheduled to be deleted")
		this.modified = true
		this.valid = true
	} else {
		spec := &this.object.DNSEntry().Spec
		targets, mappings := this.NormalizeTargets(logger, targets...)
		if len(mappings) > 0 {
			if spec.CNameLookupInterval != nil && *spec.CNameLookupInterval > 0 {
				this.interval = *spec.CNameLookupInterval
			} else {
				this.interval = 600
			}
		} else {
			this.interval = 0
		}

		ttl := defaultTTL
		if spec.TTL != nil {
			ttl = *spec.TTL
		}
		if ttl != this.ttl {
			this.ttl = ttl
			logger.Infof("setting ttl %d", ttl)
			this.modified = true
			mod.Modify(true)
			status.TTL = &ttl
		}

		if targets.DifferFrom(this.targets) {
			logger.Infof("targets differ from internal state")
			this.modified = true
			for _, w := range warnings {
				logger.Warn(w)
				this.object.Event(corev1.EventTypeNormal, "reconcile", w)
			}
			for dns, m := range mappings {
				msg := fmt.Sprintf("mapping cname %q to %v", dns, m)
				logger.Info(msg)
				this.object.Event(corev1.EventTypeNormal, "dnslookup", msg)
			}
			this.targets = targets

			list, msg := this.targetList(targets)
			if !reflect.DeepEqual(list, status.Targets) {
				this.object.Event(corev1.EventTypeNormal, "reconcile", msg)
				status.Targets = list
				logger.Info(msg)
				mod.Modify(true)
			}
		} else {
			var cur Targets
			for _, t := range status.Targets {
				cur = append(cur, NewTargetFromEntry(t, this))
			}
			if this.Targets().DifferFrom(cur) {
				status.Targets, _ = this.targetList(this.targets)
				mod.Modify(true)
			}
		}
		mod.AssureStringPtrValue(&status.Zone, zoneid)
		if err != nil {
			mod.AssureStringValue(&status.State, api.STATE_ERROR)
			mod.AssureStringPtrValue(&status.Message, err.Error())
		} else {
			if zoneid == "" {
				mod.AssureStringValue(&status.State, api.STATE_ERROR)
				mod.AssureStringPtrValue(&status.Message, fmt.Sprintf("no provider found for %q", this.dnsname))
			} else {
				if status.State != api.STATE_READY {
					mod.AssureStringValue(&status.State, api.STATE_PENDING)
				}
				if !curvalid {
					mod.AssureStringPtrValue(&status.Message, fmt.Sprintf("activating %q", this.dnsname))
					logger.Infof("activating entry for %q", this.DNSName())
					this.modified = true
				}
				this.valid = true
			}
		}
	}
	return reconcile.UpdateStatus(logger, mod.UpdateStatus())
}

func (this *Entry) targetList(targets Targets) ([]string, string) {
	list := []string{}
	msg := "update effective targets: "
	sep := "[ "
	for _, t := range targets {
		list = append(list, t.GetHostName())
		msg = fmt.Sprintf("%s%s%s", msg, sep, t)
		sep = ", "
	}
	msg = msg + "]"
	return list, msg
}

func (this *Entry) UpdateStatus(logger logger.LogContext, state string, msg string, provider resources.ObjectName) error {
	f := func(data resources.ObjectData) (bool, error) {
		o := data.(*api.DNSEntry)
		if state == api.STATE_PENDING && o.Status.State != "" {
			return false, nil
		}

		mod := &utils.ModificationState{}
		if provider != nil {
			p := provider.String()
			mod.AssureStringPtrPtr(&o.Status.Provider, &p)
		}
		mod.AssureInt64Value(&o.Status.ObservedGeneration, o.Generation)
		if !(o.Status.State == api.STATE_STALE && o.Status.State == state) {
			mod.AssureStringPtrValue(&o.Status.Message, msg)
		}
		if !(o.Status.State == api.STATE_STALE && state == api.STATE_INVALID) {
			mod.AssureStringValue(&o.Status.State, state)
		}
		if mod.IsModified() {
			logger.Infof("update state of '%s/%s' to %s (%s)", o.Namespace, o.Name, state, msg)
		}
		return mod.IsModified(), nil
	}
	_, err := this.object.ModifyStatus(f)
	return err
}

func (this *Entry) HasSameDNSName(entry *api.DNSEntry) bool {
	if this.dnsname != entry.Spec.DNSName {
		return false
	}
	return true
}

func (this *Entry) NormalizeTargets(logger logger.LogContext, targets ...Target) (Targets, map[string][]string) {

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
				this.object.Event(corev1.EventTypeNormal, "dnslookup", w)
			}
			mappings[t.GetHostName()] = addrs
		} else {
			result = append(result, t)
		}
	}
	return result, mappings
}

func (this *Entry) Before(e *Entry) bool {
	if e == nil {
		return true
	}
	if this.object.GetCreationTimestamp().Time.Equal(e.object.GetCreationTimestamp().Time) {
		// for entries created at same time compare objectname to define strict order
		return strings.Compare(this.object.ObjectName().String(), e.object.ObjectName().String()) < 0
	}
	return this.object.GetCreationTimestamp().Time.Before(e.object.GetCreationTimestamp().Time)
}

////////////////////////////////////////////////////////////////////////////////
// Entries
////////////////////////////////////////////////////////////////////////////////

type Entries map[resources.ObjectName]*Entry

// Add an entry. It preserves the existing entry if the DNS name is
// unchanged. Uf the name changes the enetry will be replaced by a new one.
// In this case the old entry (with the previoud DNS name) is returned
// as old entry. In all other cases no old entry is returned.
// return 1: old entry with different dns name
// return 2: actual entry for this object name with the actual dns name
func (this Entries) Add(entry *dnsutils.DNSEntryObject) (*Entry, *Entry) {
	data := entry.DNSEntry()
	old := this[entry.ObjectName()]
	if old != nil && old.HasSameDNSName(data) {
		return nil, old
	}
	e := NewEntry(entry)
	this[entry.ObjectName()] = e
	return old, e
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
