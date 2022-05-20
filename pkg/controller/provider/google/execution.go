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

package google

import (
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
	googledns "google.golang.org/api/dns/v1"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const (
	googleRecordTTL = 300
)

type Execution struct {
	logger.LogContext
	handler *Handler
	zone    provider.DNSHostedZone

	change *googledns.Change
	done   []provider.DoneHandler
}

func NewExecution(logger logger.LogContext, h *Handler, zone provider.DNSHostedZone) *Execution {
	change := &googledns.Change{
		Additions: []*googledns.ResourceRecordSet{},
		Deletions: []*googledns.ResourceRecordSet{},
	}
	return &Execution{
		LogContext: logger,
		handler:    h,
		zone:       zone,
		change:     change,
		done:       []provider.DoneHandler{},
	}
}

func (this *Execution) addChange(req *provider.ChangeRequest) {
	var name string
	var newset, oldset *dns.RecordSet

	if req.Addition != nil {
		name, newset = dns.MapToProvider(req.Type, req.Addition, this.zone.Domain())
	}
	if req.Deletion != nil {
		name, oldset = dns.MapToProvider(req.Type, req.Deletion, this.zone.Domain())
	}
	if name == "" || (newset.Length() == 0 && oldset.Length() == 0) {
		return
	}
	name = dns.AlignHostname(name)
	switch req.Action {
	case provider.R_CREATE:
		this.Infof("%s %s record set %s[%s]: %s(%d)", req.Action, req.Type, name, this.zone.Id(), newset.RecordString(), newset.TTL)
		this.change.Additions = append(this.change.Additions, mapRecordSet(name, newset))
	case provider.R_DELETE:
		this.Infof("%s %s record set %s[%s]: %s", req.Action, req.Type, name, this.zone.Id(), oldset.RecordString())
		this.change.Deletions = append(this.change.Deletions, mapRecordSet(name, oldset))
	case provider.R_UPDATE:
		this.Infof("%s %s record set %s[%s]: %s(%d)", req.Action, req.Type, name, this.zone.Id(), newset.RecordString(), newset.TTL)
		this.change.Deletions = append(this.change.Deletions, mapRecordSet(name, oldset))
		this.change.Additions = append(this.change.Additions, mapRecordSet(name, newset))
	}
	this.done = append(this.done, req.Done)
}

func (this *Execution) submitChanges(metrics provider.Metrics) error {
	if len(this.change.Additions) == 0 && len(this.change.Deletions) == 0 {
		return nil
	}

	this.Infof("processing changes for  zone %s", this.zone.Id())
	for _, c := range this.change.Deletions {
		this.Infof("desired change: Deletion %s %s: %s", c.Name, c.Type, utils.Strings(c.Rrdatas...))
	}
	for _, c := range this.change.Additions {
		this.Infof("desired change: Addition %s %s: %s", c.Name, c.Type, utils.Strings(c.Rrdatas...))
	}

	metrics.AddZoneRequests(this.zone.Id().ID, provider.M_UPDATERECORDS, 1)
	this.handler.config.RateLimiter.Accept()
	projectID, zoneName := SplitZoneID(this.zone.Id().ID)
	if _, err := this.handler.service.Changes.Create(projectID, zoneName, this.change).Do(); err != nil {
		this.Error(err)
		for _, d := range this.done {
			if d != nil {
				d.Failed(err)
			}
		}
		return err
	} else {
		for _, d := range this.done {
			if d != nil {
				d.Succeeded()
			}
		}
		this.Infof("%d records in zone %s were successfully updated", len(this.change.Additions)+len(this.change.Deletions), this.zone.Id())
		return nil
	}
}

func mapRecordSet(dnsname string, rs *dns.RecordSet) *googledns.ResourceRecordSet {
	targets := make([]string, len(rs.Records))
	for i, r := range rs.Records {
		if rs.Type == dns.RS_CNAME {
			targets[i] = dns.AlignHostname(r.Value)
		} else {
			targets[i] = r.Value
		}
	}

	// no annotation results in a TTL of 0, default to 300 for backwards-compatibility
	var ttl int64 = googleRecordTTL
	if rs.TTL > 0 {
		ttl = rs.TTL
	}

	return &googledns.ResourceRecordSet{
		Name:    dnsname,
		Rrdatas: targets,
		Ttl:     ttl,
		Type:    rs.Type,
	}
}
