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

package alicloud

import (
	"fmt"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const (
	googleRecordTTL = 300
)

type result struct {
	done []provider.DoneHandler
	err  error
}

type Execution struct {
	logger.LogContext
	handler *Handler
	zone    provider.DNSHostedZone
	state   *zonestate
	domain  string

	additions RecordSet
	updates   RecordSet
	deletions RecordSet

	results map[string]*result
}

func NewExecution(logger logger.LogContext, h *Handler, state *zonestate, zone provider.DNSHostedZone) *Execution {
	return &Execution{
		LogContext: logger,
		handler:    h,
		zone:       zone,
		state:      state,
		results:    map[string]*result{},
		additions:  RecordSet{},
		updates:    RecordSet{},
		deletions:  RecordSet{},
	}
}

func (this *Execution) addChange(req *provider.ChangeRequest) {
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
	rr := GetRR(name, this.zone.Domain())

	switch req.Action {
	case provider.R_CREATE:
		this.Infof("%s %s record set %s[%s]: %s(%d)", req.Action, req.Type, name, this.zone.Id(), newset.RecordString(), newset.TTL)
		this.add(name, rr, newset, &this.updates, &this.additions)
	case provider.R_DELETE:
		this.Infof("%s %s record set %s[%s]: %s", req.Action, req.Type, name, this.zone.Id(), oldset.RecordString())
		this.add(name, rr, oldset, &this.deletions, nil)
	case provider.R_UPDATE:
		this.Infof("%s %s record set %s[%s]: %s(%d)", req.Action, req.Type, name, this.zone.Id(), newset.RecordString(), newset.TTL)
		this.add(name, rr, newset, &this.updates, &this.additions)
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

func (this *Execution) add(dnsname, rr string, rset *dns.RecordSet, found *RecordSet, notfound *RecordSet) {
	rtype := rset.Type
	for _, r := range rset.Records {
		old := this.state.getRecord(dnsname, rtype, r.Value)
		if old != nil {
			or := *old
			or.TTL = int(rset.TTL)
			*found = append(*found, or)
		} else {
			if notfound != nil {
				nr := alidns.Record{RR: rr, Type: rtype, Value: r.Value, DomainName: this.zone.Domain(), TTL: int(rset.TTL)}
				*notfound = append(*notfound, nr)
			}
		}
	}
}

func (this *Execution) submitChanges() error {

	if len(this.additions) == 0 && len(this.updates) == 0 && len(this.deletions) == 0 {
		return nil
	}

	this.Infof("processing changes for  zone %s", this.zone.Id())
	for _, r := range this.additions {
		this.Infof("desired change: Addition %s %s: %s (%d)", GetDNSName(r), r.Type, r.Value, r.TTL)
		this.submit(this.handler.access.CreateRecord, r)
	}
	for _, r := range this.updates {
		this.Infof("desired change: Update %s %s: %s (%d)", GetDNSName(r), r.Type, r.Value, r.TTL)
		this.submit(this.handler.access.UpdateRecord, r)
	}
	for _, r := range this.deletions {
		this.Infof("desired change: Deletion %s %s: %s", GetDNSName(r), r.Type, r.Value)
		this.submit(this.handler.access.DeleteRecord, r)
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
		this.Infof("%d records in zone %s were successfully updated", suc_cnt, this.zone.Id())
	}
	if err_cnt > 0 {
		this.Infof("%d records in zone %s were successfully updated", err_cnt, this.zone.Id())
		return fmt.Errorf("could not update all dns entries")
	}
	return nil
}

func (this *Execution) submit(f func(record alidns.Record) error, r alidns.Record) {
	err := f(r)
	if err != nil {
		r := this.results[GetDNSName(r)]
		if r != nil {
			r.err = err
		}
	}
}
