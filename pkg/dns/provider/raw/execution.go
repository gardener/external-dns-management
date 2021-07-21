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

package raw

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Executor interface {
	CreateRecord(r Record, zone provider.DNSHostedZone) error
	UpdateRecord(r Record, zone provider.DNSHostedZone) error
	DeleteRecord(r Record, zone provider.DNSHostedZone) error

	NewRecord(fqdn, rtype, value string, zone provider.DNSHostedZone, ttl int64) Record
}

type result struct {
	done []provider.DoneHandler
	err  error
}

type Execution struct {
	logger.LogContext
	executor Executor
	zone     provider.DNSHostedZone
	state    *ZoneState
	domain   string

	additions RecordSet
	updates   RecordSet
	deletions RecordSet

	results map[string]*result
}

func NewExecution(logger logger.LogContext, e Executor, state *ZoneState, zone provider.DNSHostedZone) *Execution {
	return &Execution{
		LogContext: logger,
		executor:   e,
		zone:       zone,
		state:      state,
		domain:     zone.Domain(),
		results:    map[string]*result{},
		additions:  RecordSet{},
		updates:    RecordSet{},
		deletions:  RecordSet{},
	}
}

func (this *Execution) AddChange(req *provider.ChangeRequest) {
	var name string
	var newset, oldset *dns.RecordSet

	if req.Addition != nil {
		name, newset = dns.MapToProvider(req.Type, req.Addition, this.domain)
	}
	if req.Deletion != nil {
		name, oldset = dns.MapToProvider(req.Type, req.Deletion, this.domain)
	}
	if name == "" || (newset.Length() == 0 && oldset.Length() == 0) {
		return
	}
	switch req.Action {
	case provider.R_CREATE:
		this.Infof("%s %s record set %s[%s]: %s(%d)", req.Action, req.Type, name, this.zone.Id(), newset.RecordString(), newset.TTL)
		this.add(name, newset, true, &this.updates, &this.additions)
	case provider.R_DELETE:
		this.Infof("%s %s record set %s[%s]: %s", req.Action, req.Type, name, this.zone.Id(), oldset.RecordString())
		this.add(name, oldset, false, &this.deletions, nil)
	case provider.R_UPDATE:
		this.Infof("%s %s record set %s[%s]: %s(%d)", req.Action, req.Type, name, this.zone.Id(), newset.RecordString(), newset.TTL)
		if oldset != nil {
			_, _, del := newset.DiffTo(oldset)
			if len(del) > 0 {
				this.add(name, dns.NewRecordSet(oldset.Type, oldset.TTL, del), false, &this.deletions, nil)
			}
		}
		this.add(name, newset, true, &this.updates, &this.additions)
	}

	r := this.results[name]
	if r == nil {
		r = &result{}
		this.results[name] = r
	}
	if req.Done != nil {
		r.done = append(r.done, req.Done)
	}
}

func (this *Execution) add(dnsname string, rset *dns.RecordSet, modonly bool, found *RecordSet, notfound *RecordSet) {
	rtype := rset.Type
	for _, r := range rset.Records {
		old := this.state.GetRecord(dnsname, rtype, r.Value)
		if old != nil {
			if (!modonly) || (old.GetTTL() != int(rset.TTL)) {
				or := old.Copy()
				or.SetTTL(int(rset.TTL))
				*found = append(*found, or)
			}
		} else {
			if notfound != nil {
				record := this.executor.NewRecord(dnsname, rset.Type, r.Value, this.zone, rset.TTL)
				*notfound = append(*notfound, record)
			}
		}
	}
}

func (this *Execution) SubmitChanges() error {

	if len(this.additions) == 0 && len(this.updates) == 0 && len(this.deletions) == 0 {
		return nil
	}

	this.Infof("processing changes for zone %s", this.zone.Id())
	for _, r := range this.additions {
		this.Infof("desired change: Addition %s %s: %s (%d)", r.GetDNSName(), r.GetType(), r.GetValue(), r.GetTTL())
		this.submit(this.executor.CreateRecord, r)
	}
	for _, r := range this.updates {
		this.Infof("desired change: Update %s %s: %s (%d)", r.GetDNSName(), r.GetType(), r.GetValue(), r.GetTTL())
		this.submit(this.executor.UpdateRecord, r)
	}
	for _, r := range this.deletions {
		this.Infof("desired change: Deletion %s %s: %s", r.GetDNSName(), r.GetType(), r.GetValue())
		this.submit(this.executor.DeleteRecord, r)
	}

	err_cnt := 0
	suc_cnt := 0
	for _, r := range this.results {
		if r.err != nil {
			err_cnt++
			for _, d := range r.done {
				d.Failed(r.err)
			}
		} else {
			suc_cnt++
			for _, d := range r.done {
				d.Succeeded()
			}
		}
	}

	if suc_cnt > 0 {
		this.Infof("record sets for %d names in zone %s were successfully updated", suc_cnt, this.zone.Id())
	}
	if err_cnt > 0 {
		this.Infof("record sets for %d names in zone %s failed", err_cnt, this.zone.Id())
		return fmt.Errorf("could not update all dns entries")
	}
	return nil
}

func (this *Execution) submit(f func(record Record, zone provider.DNSHostedZone) error, r Record) {
	err := f(r, this.zone)
	if err != nil {
		res := this.results[r.GetDNSName()]
		if res != nil {
			res.err = err
			this.Infof("operation failed for %s %s: %s", r.GetType(), r.GetDNSName(), err)
		}
	}
}

func ExecuteRequests(logger logger.LogContext, config *provider.DNSHandlerConfig, e Executor, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, e, state.(*ZoneState), zone)
	for _, r := range reqs {
		exec.AddChange(r)
	}
	if config.DryRun {
		logger.Infof("no changes in dryrun mode")
		return nil
	}
	return exec.SubmitChanges()
}
