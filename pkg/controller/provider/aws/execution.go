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

package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"

	"github.com/gardener/controller-manager-library/pkg/logger"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Change struct {
	*route53.Change
	Done provider.DoneHandler
}

type Execution struct {
	logger.LogContext
	handler *Handler
	zone    provider.DNSHostedZone

	changes        map[string][]*Change
	maxChangeCount int
}

func NewExecution(logger logger.LogContext, h *Handler, zone provider.DNSHostedZone) *Execution {
	return &Execution{LogContext: logger, handler: h, zone: zone, changes: map[string][]*Change{}, maxChangeCount: 20}
}

func (this *Execution) addChange(action string, req *provider.ChangeRequest, dnsset *dns.DNSSet) {
	name, rset := dns.MapToProvider(req.Type, dnsset, this.zone.Domain())
	name = dns.AlignHostname(name)
	if len(rset.Records) == 0 {
		return
	}
	this.Infof("%s %s record set %s[%s]: %s(%d)", action, rset.Type, name, this.zone.Id(), rset.RecordString(), rset.TTL)
	change := &route53.Change{
		Action: aws.String(action),
		ResourceRecordSet: &route53.ResourceRecordSet{
			Name: aws.String(name),
		},
	}

	change.ResourceRecordSet.Type = aws.String(rset.Type)
	change.ResourceRecordSet.TTL = aws.Int64(rset.TTL)
	change.ResourceRecordSet.ResourceRecords = make([]*route53.ResourceRecord, len(rset.Records))
	for i, r := range rset.Records {
		change.ResourceRecordSet.ResourceRecords[i] = &route53.ResourceRecord{
			Value: aws.String(r.Value),
		}
	}

	this.changes[name] = append(this.changes[name], &Change{Change: change, Done: req.Done})
}

func (this *Execution) submitChanges() error {
	if len(this.changes) == 0 {
		return nil
	}

	limitedChanges := limitChangeSet(this.changes, this.maxChangeCount)
	for i, changes := range limitedChanges {
		this.Infof("processing batch %d for zone %s", i+1, this.zone.Id())
		for _, c := range changes {
			this.Infof("desired change: %s %s %s", *c.Action, *c.ResourceRecordSet.Name, *c.ResourceRecordSet.Type)
		}

		params := &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(this.zone.Id()),
			ChangeBatch: &route53.ChangeBatch{
				Changes: mapChanges(changes),
			},
		}

		if _, err := this.handler.r53.ChangeResourceRecordSets(params); err != nil {
			this.Error(err)
			for _, c := range changes {
				if c.Done != nil {
					c.Done.Failed(err)
				}
			}
			continue
		} else {
			for _, c := range changes {
				if c.Done != nil {
					c.Done.Succeeded()
				}
			}
			this.Infof("%d records in zone %s were successfully updated", len(changes), this.zone.Id())
		}
	}
	return nil
}

func limitChangeSet(changesByName map[string][]*Change, max int) [][]*Change {
	batches := [][]*Change{}

	// add deleteion requests
	batch := make([]*Change, 0)
	for _, changes := range changesByName {
		for _, change := range changes {
			if aws.StringValue(change.Change.Action) == route53.ChangeActionDelete {
				batch = addLimited(change, batch, batches, max)
			}
		}
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
		batch = make([]*Change, 0)
	}

	// add non-deletion requests

	for _, changes := range changesByName {
		for _, change := range changes {
			if aws.StringValue(change.Change.Action) != route53.ChangeActionDelete {
				batch = addLimited(change, batch, batches, max)
			}
		}
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}

	return batches
}

func addLimited(change *Change, batch []*Change, batches [][]*Change, max int) []*Change {
	if len(batch) >= max {
		batches = append(batches, batch)
		batch = make([]*Change, 0)
	}
	return append(batch, change)
}

func mapChanges(changes []*Change) []*route53.Change {
	dest := []*route53.Change{}
	for _, c := range changes {
		dest = append(dest, c.Change)
	}
	return dest
}
