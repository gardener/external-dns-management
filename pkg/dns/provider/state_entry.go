/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package provider

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

////////////////////////////////////////////////////////////////////////////////
// state handling for entries
////////////////////////////////////////////////////////////////////////////////

func (this *state) IsManaging(v *EntryVersion) bool {
	if v.status.ProviderType == nil {
		return false
	}
	return this.GetHandlerFactory().TypeCodes().Contains(*v.status.ProviderType)
}

func (this *state) TriggerEntries(logger logger.LogContext, entries Entries) {
	for _, e := range entries {
		this.TriggerEntry(logger, e)
	}
}

func (this *state) TriggerEntry(logger logger.LogContext, e *Entry) {
	if logger != nil {
		logger.Infof("trigger entry %s", e.ClusterKey())
	}
	this.context.EnqueueKey(e.ClusterKey())
}

func (this *state) TriggerEntriesByOwner(logger logger.LogContext, owners utils.StringSet) {
	for _, e := range this.GetEntriesByOwner(owners) {
		this.TriggerEntry(logger, e)
	}
}

func (this *state) GetEntriesByOwner(owners utils.StringSet) Entries {
	if len(owners) == 0 {
		return nil
	}
	this.lock.RLock()
	defer this.lock.RUnlock()

	entries := Entries{}
	for _, e := range this.entries {
		if owners.Contains(e.OwnerId()) {
			entries[e.ObjectName()] = e
		}
	}
	return entries
}

func (this *state) addBlockingEntries(logger logger.LogContext, entries Entries) {
	if len(entries) == 0 {
		return
	}
	logger.Infof("blocking hosted zone reconciliation for %d entries", len(entries))
	now := time.Now()
	for _, e := range entries {
		if _, ok := this.blockingEntries[e.ObjectName()]; !ok {
			this.blockingEntries[e.ObjectName()] = now
		}
	}
}

////////////////////////////////////////////////////////////////////////////////

func (this *state) UpdateEntry(logger logger.LogContext, object dnsutils.DNSSpecification) reconcile.Status {
	return this.HandleUpdateEntry(logger, "reconcile", object)
}

func (this *state) DeleteEntry(logger logger.LogContext, object dnsutils.DNSSpecification) reconcile.Status {
	return this.HandleUpdateEntry(logger, "delete", object)
}

func (this *state) GetEntry(name resources.ObjectName) *Entry {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.entries[name]
}

func (this *state) SmartInfof(logger logger.LogContext, format string, args ...interface{}) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	this.smartInfof(logger, format, args...)
}

func (this *state) smartInfof(logger logger.LogContext, format string, args ...interface{}) {
	if this.hasProviders() {
		logger.Infof(format, args...)
	} else {
		logger.Debugf(format, args...)
	}
}

func (this *state) AddEntryVersion(logger logger.LogContext, v *EntryVersion, status reconcile.Status) (*Entry, reconcile.Status) {
	this.lock.Lock()
	defer this.lock.Unlock()

	delete(this.blockingEntries, v.ObjectName())

	var new *Entry
	old := this.entries[v.ObjectName()]
	if old == nil {
		new = NewEntry(v, this)
	} else {
		new = old.Update(logger, v)
	}

	if v.IsDeleting() {
		var err error
		if old != nil {
			if old.Kind() != api.DNSLockKind { // TODO: why is cleanup called here
				this.cleanupEntry(logger, old)
			}
		}
		if new.valid {
			if this.zones[new.activezone] != nil {
				if this.HasFinalizer(new.Object()) {
					logger.Infof("deleting delayed until entry deleted in provider")
					this.outdated.AddEntry(new)
					return new, reconcile.Succeeded(logger)
				}
			} else {
				if old != nil {
					logger.Infof("dns zone '%s' of deleted entry gone", old.ZoneId())
				}
				if !new.IsActive() || v.object.BaseStatus().Zone == nil {
					err = this.RemoveFinalizer(v.object)
				}
			}
		} else {
			if !new.IsActive() || v.object.BaseStatus().State != api.STATE_STALE {
				this.smartInfof(logger, "deleting yet unmanaged or errorneous entry")
				err = this.RemoveFinalizer(v.object)
			} else {
				if this.HasFinalizer(v.object) {
					this.smartInfof(logger, "preventing deletion of stale entry")
				}
			}
		}
		if err != nil {
			this.entries[v.ObjectName()] = new
		}
		return new, reconcile.DelayOnError(logger, err)
	}

	if new.valid && this.IsManaging(v) {
		err := this.SetFinalizer(new.Object())
		if err != nil {
			return new, reconcile.DelayOnError(logger, err)
		}
	}
	this.entries[v.ObjectName()] = new

	if old != nil && old != new {
		// DNS name changed -> clean up old dns name
		logger.Infof("dns name changed to %q", new.DNSName())
		this.cleanupEntry(logger, old)
		if old.activezone != "" && old.activezone != new.ZoneId() {
			if this.zones[old.activezone] != nil {
				logger.Infof("dns zone changed -> trigger old zone '%s'", old.ZoneId())
				this.triggerHostedZone(old.activezone)
			}
		}
	}

	if !this.IsManaging(v) {
		this.smartInfof(logger, "foreign zone %s(%s) -> skip reconcilation", utils.StringValue(v.status.Zone), utils.StringValue(v.status.ProviderType))
		return nil, status
	}

	dnsname := v.DNSName()
	cur := this.dnsnames[dnsname]
	if dnsname != "" {
		if cur != nil {
			if cur.ObjectName() != new.ObjectName() {
				if cur.Before(new) {
					new.duplicate = true
					new.modified = false
					err := &perrs.AlreadyBusyForEntry{DNSName: dnsname, ObjectName: cur.ObjectName()}
					logger.Warnf("%s", err)
					if status.IsSucceeded() {
						_, err := v.UpdateStatus(logger, api.STATE_ERROR, err.Error())
						if err != nil {
							return new, reconcile.DelayOnError(logger, err)
						}
					}
					return new, status
				} else {
					cur.duplicate = true
					cur.modified = false
					logger.Warnf("DNS name %q already busy for entry %q, but this one was earlier", dnsname, cur.ObjectName())
					logger.Infof("reschedule %q for error update", cur.ObjectName())
					this.triggerKey(cur.ClusterKey())
				}
			}
		}
		if new.valid && new.status.State != api.STATE_READY && new.status.State != api.STATE_PENDING {
			msg := fmt.Sprintf("activating for %s", new.DNSName())
			logger.Info(msg)
			_, err := new.UpdateStatus(logger, api.STATE_PENDING, msg)
			if err != nil {
				logger.Errorf("cannot update: %s", err)
			}
		}

		this.dnsnames[dnsname] = new
	}

	return new, status
}

func (this *state) EntryPremise(e dnsutils.DNSSpecification) (*EntryPremise, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	provider, fallback, err := this.lookupProvider(e)
	p := &EntryPremise{
		ptypes:   this.config.Enabled,
		provider: provider,
		fallback: fallback,
	}
	zone := this.getProviderZoneForName(e.GetDNSName(), provider)

	if zone != nil {
		p.ptype = zone.ProviderType()
		p.zoneid = zone.Id()
		p.zonedomain = zone.Domain()
	} else if provider != nil && !provider.IsValid() && e.BaseStatus().Zone != nil {
		p.ptype = provider.TypeCode()
		p.zoneid = *e.BaseStatus().Zone
	} else if p.fallback != nil {
		zone = this.getProviderZoneForName(e.GetDNSName(), p.fallback)
		if zone != nil {
			p.ptype = zone.ProviderType()
			p.zoneid = zone.Id()
			p.zonedomain = zone.Domain()
		}
	}
	return p, err
}

func (this *state) HandleUpdateEntry(logger logger.LogContext, op string, object dnsutils.DNSSpecification) reconcile.Status {
	old := this.GetEntry(object.ObjectName())
	if old != nil {
		old.lock.Lock()
		defer old.lock.Unlock()
	}

	p, err := this.EntryPremise(object)
	if p.provider == nil && err == nil {
		if p.zoneid != "" {
			err = fmt.Errorf("no matching provider for zone '%s' found", p.zoneid)
		}
	}

	defer this.triggerStatistic()
	defer this.references.NotifyHolder(this.context, object.ClusterKey())

	logger = this.RefineLogger(logger, p.ptype)
	v := NewEntryVersion(object, old)
	if p.fallback != nil {
		v.obsolete = true
	}
	status := v.Setup(logger, this, p, op, err, this.config, old)
	new, status := this.AddEntryVersion(logger, v, status)

	if new != nil {
		if status.IsSucceeded() && new.IsValid() {
			if new.Interval() > 0 {
				status = status.RescheduleAfter(time.Duration(new.Interval()) * time.Second)
			}
		}
		if new.IsModified() && new.ZoneId() != "" {
			this.SmartInfof(logger, "trigger zone %q", new.ZoneId())
			this.TriggerHostedZone(new.ZoneId())
		} else {
			logger.Debugf("skipping trigger zone %q because entry not modified", new.ZoneId())
		}
	}

	if !object.IsDeleting() {
		check, _ := this.EntryPremise(object)
		if !check.Match(p) {
			logger.Infof("%s -> repeat reconcilation", p.NotifyChange(check))
			return reconcile.Repeat(logger)
		}
	}
	return status
}

func (this *state) EntryDeleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	this.lock.Lock()
	defer func() {
		this.lock.Unlock()
		this.references.DelRef(key)
		this.references.NotifyHolder(this.context, key)
	}()

	delete(this.blockingEntries, key.ObjectName())

	old := this.entries[key.ObjectName()]
	if old != nil {
		provider, _, _ := this.lookupProvider(old.object)
		zone := this.getProviderZoneForName(old.DNSName(), provider)
		if zone != nil {
			logger.Infof("removing entry %q (%s[%s])", key.ObjectName(), old.DNSName(), zone.Id())
			this.triggerHostedZone(zone.Id())
		} else {
			this.smartInfof(logger, "removing foreign entry %q (%s)", key.ObjectName(), old.DNSName())
		}
		this.cleanupEntry(logger, old)
	} else {
		logger.Debugf("removing unknown entry %q", key.ObjectName())
	}
	return reconcile.Succeeded(logger)
}

func (this *state) cleanupEntry(logger logger.LogContext, e *Entry) {
	this.smartInfof(logger, "cleanup old entry (duplicate=%t)", e.duplicate)
	this.entries.Delete(e)
	if this.dnsnames[e.DNSName()] == e {
		var found *Entry
		for _, a := range this.entries {
			logger.Debugf("  checking %s(%s): dup:%t", a.ObjectName(), a.DNSName(), a.duplicate)
			if a.duplicate && a.DNSName() == e.DNSName() {
				if found == nil {
					found = a
				} else {
					if a.Before(found) {
						found = a
					}
				}
			}
		}
		if found == nil {
			logger.Infof("no duplicate found to reactivate")
		} else {
			old := this.dnsnames[found.DNSName()]
			msg := ""
			if old != nil {
				msg = fmt.Sprintf("reactivate duplicate for %s: %s replacing %s", found.DNSName(), found.ObjectName(), e.ObjectName())
			} else {
				msg = fmt.Sprintf("reactivate duplicate for %s: %s", found.DNSName(), found.ObjectName())
			}
			logger.Info(msg)
			found.Trigger(nil)
		}
		delete(this.dnsnames, e.DNSName())
	}
}

func (this *state) UpdateLockStates(log logger.LogContext) {
	this.lock.RLock()
	entries := Entries{}
	for _, e := range this.entries {
		if e.Kind() == api.DNSLockKind {
			entries.AddEntry(e)
		}
	}
	this.lock.RUnlock()

	for _, e := range entries {
		ts := time.Time{}
		attrs := map[string]string{}
		records, err := net.LookupTXT(e.DNSName())
		if err == nil {
			log.Infof("found records %v", records)
			for _, r := range records {
				r = strings.Trim(r, "\"")
				fields := strings.Split(r, "=")
				if len(fields) != 2 {
					continue
				}
				if fields[0] == dns.ATTR_TIMESTAMP {
					i, err := strconv.ParseInt(fields[1], 10, 64)
					if err != nil {
						continue
					}
					ts = time.Unix(i, 0)
				} else {
					attrs[fields[0]] = fields[1]
				}
			}
		} else {
			log.Warnf("dns lookup failed for %q: %s", e.DNSName(), err)
		}

		e.object.ModifyStatus(func(data resources.ObjectData) (bool, error) {
			status := &data.(*api.DNSLock).Status
			mod := false
			if ts.IsZero() {
				mod = status.Timestamp == nil
				status.Timestamp = nil
			} else {
				if status.Timestamp == nil || !status.Timestamp.Time.Equal(ts) {
					mod = true
				}
				t := metav1.NewTime(ts)
				status.Timestamp = &t
			}
			mod = mod || !EqualAttrs(attrs, status.Attributes)
			status.Attributes = attrs
			return mod, nil
		})
	}
}

func EqualAttrs(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	if a == nil || b == nil {
		return false
	}
	for k, v := range a {
		if f, ok := b[k]; !ok || f != v {
			return false
		}
	}
	return true
}
