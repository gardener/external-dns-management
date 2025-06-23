// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package google

import (
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
	googledns "google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"

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

	routingPolicyChanges routingPolicyChanges
}

func NewExecution(logger logger.LogContext, h *Handler, zone provider.DNSHostedZone) *Execution {
	change := &googledns.Change{
		Additions: []*googledns.ResourceRecordSet{},
		Deletions: []*googledns.ResourceRecordSet{},
	}
	return &Execution{
		LogContext:           logger,
		handler:              h,
		zone:                 zone,
		change:               change,
		done:                 []provider.DoneHandler{},
		routingPolicyChanges: routingPolicyChanges{},
	}
}

func (this *Execution) addChange(req *provider.ChangeRequest) {
	var setName dns.DNSSetName
	var newset, oldset *dns.RecordSet
	var policy *googleRoutingPolicyData
	var err error

	if req.Addition != nil {
		setName = req.Addition.Name
		newset = req.Addition.Sets[req.Type]
		policy, err = extractRoutingPolicy(req.Addition)
	}
	if req.Deletion != nil {
		setName = req.Deletion.Name
		oldset = req.Deletion.Sets[req.Type]
		if req.Addition == nil {
			policy, err = extractRoutingPolicy(req.Deletion)
		}
	}
	if err != nil {
		if req.Done != nil {
			req.Done.SetInvalid(err)
		}
		return
	}

	if setName.DNSName == "" || (newset.Length() == 0 && oldset.Length() == 0) {
		return
	}
	setName = setName.Align()
	switch req.Action {
	case provider.R_CREATE:
		this.Infof("%s %s record set %s[%s]: %s(%d)", req.Action, req.Type, setName, this.zone.Id(), newset.RecordString(), newset.TTL)
		this.addAddition(mapRecordSet(setName, newset, policy), req.Done)
	case provider.R_DELETE:
		this.Infof("%s %s record set %s[%s]: %s", req.Action, req.Type, setName, this.zone.Id(), oldset.RecordString())
		this.addDeletion(mapRecordSet(setName, oldset, policy), req.Done)
	case provider.R_UPDATE:
		this.Infof("%s %s record set %s[%s]: %s(%d)", req.Action, req.Type, setName, this.zone.Id(), newset.RecordString(), newset.TTL)
		this.addDeletion(mapRecordSet(setName, oldset, policy), req.Done)
		this.addAddition(mapRecordSet(setName, newset, policy), nil)
	}
}

func (this *Execution) addAddition(set *googledns.ResourceRecordSet, done provider.DoneHandler) {
	if done != nil {
		this.done = append(this.done, done)
	}
	if set.RoutingPolicy == nil {
		this.change.Additions = append(this.change.Additions, set)
		return
	}

	this.routingPolicyChanges.addChange(set, true)
}

func (this *Execution) addDeletion(set *googledns.ResourceRecordSet, done provider.DoneHandler) {
	if done != nil {
		this.done = append(this.done, done)
	}
	if set.RoutingPolicy == nil {
		this.change.Deletions = append(this.change.Deletions, set)
		return
	}

	this.routingPolicyChanges.addChange(set, false)
}

func (this *Execution) prepareSubmission(rrsetGetter rrsetGetterFunc) error {
	routingPolicyDeletions, routingPolicyAdditions, err := this.routingPolicyChanges.calcDeletionsAndAdditions(rrsetGetter)
	if err != nil {
		return err
	}

	for _, c := range this.change.Deletions {
		this.Infof("desired change: Deletion %s %s: %s", c.Name, c.Type, utils.Strings(c.Rrdatas...))
	}
	for _, c := range routingPolicyDeletions {
		this.Infof("desired change: Deletion %s %s (routing policy: %s)", c.Name, c.Type, describeRoutingPolicy(c))
		this.change.Deletions = append(this.change.Deletions, c)
	}
	for _, c := range this.change.Additions {
		this.Infof("desired change: Addition %s %s: %s", c.Name, c.Type, utils.Strings(c.Rrdatas...))
	}
	for _, c := range routingPolicyAdditions {
		this.Infof("desired change: Addition %s %s (routing policy: %s)", c.Name, c.Type, describeRoutingPolicy(c))
		this.change.Additions = append(this.change.Additions, c)
	}
	return nil
}

func (this *Execution) submitChanges(metrics provider.Metrics) error {
	if len(this.change.Additions) == 0 && len(this.change.Deletions) == 0 && len(this.routingPolicyChanges) == 0 {
		return nil
	}

	this.Infof("processing changes for  zone %s", this.zone.Id())
	projectID, zoneName := SplitZoneID(this.zone.Id().ID)
	rrsetGetter := func(name, typ string) (*googledns.ResourceRecordSet, error) {
		return this.handler.getResourceRecordSet(projectID, zoneName, name, typ)
	}
	err := this.prepareSubmission(rrsetGetter)
	if err != nil {
		this.Error(err)
		for _, d := range this.done {
			if d != nil {
				d.Failed(err)
			}
		}
		return err
	}

	metrics.AddZoneRequests(this.zone.Id().ID, provider.M_UPDATERECORDS, 1)
	this.handler.config.RateLimiter.Accept()
	if _, err := this.handler.service.Changes.Create(projectID, zoneName, this.change).Do(); err != nil {
		this.Error(err)
		for _, d := range this.done {
			if d != nil {
				d.Failed(err)
			}
		}
		return err
	}

	for _, d := range this.done {
		if d != nil {
			d.Succeeded()
		}
	}
	this.Infof("%d records in zone %s were successfully updated", len(this.change.Additions)+len(this.change.Deletions), this.zone.Id())
	return nil
}

func isNotFound(err error) bool {
	if ge, ok := err.(*googleapi.Error); ok {
		return ge.Code == 404
	}
	return false
}

func mapRecordSet(name dns.DNSSetName, rs *dns.RecordSet, policy *googleRoutingPolicyData) *googledns.ResourceRecordSet {
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

	rrset := &googledns.ResourceRecordSet{
		Name:    name.DNSName,
		Rrdatas: targets,
		Ttl:     ttl,
		Type:    rs.Type,
	}
	rrset = mapPolicyRecordSet(rrset, policy)
	return rrset
}
