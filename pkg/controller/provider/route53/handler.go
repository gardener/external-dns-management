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

package route53

import (
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/gardener/external-dns-management/pkg/dns"
)

type Handler struct {
	config provider.DNSHandlerConfig

	sess *session.Session
	r53  *route53.Route53
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(logger logger.LogContext, config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	this := &Handler{
		config: *config,
	}
	akid := this.config.Properties["AWS_ACCESS_KEY_ID"]
	if akid == "" {
		return nil, fmt.Errorf("'AWS_ACCESS_KEY_ID' required in secret")
	}
	sak := this.config.Properties["AWS_SECRET_ACCESS_KEY"]
	if sak == "" {
		return nil, fmt.Errorf("'AWS_SECRET_ACCESS_KEY' required in secret")
	}
	st := this.config.Properties["AWS_SESSION_TOKEN"]
	creds := credentials.NewStaticCredentials(akid, sak, st)

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-west-2"),
		Credentials: creds,
	})
	if err != nil {
		return nil, err
	}
	this.sess = sess
	this.r53 = route53.New(sess)
	return this, nil
}

func (this *Handler) GetZones() (provider.DNSHostedZones, error) {
	raw := []*route53.HostedZone{}
	aggr := func(resp *route53.ListHostedZonesOutput, lastPage bool) bool {
		for _, zone := range resp.HostedZones {
			raw = append(raw, zone)
		}
		return true
	}

	err := this.r53.ListHostedZonesPages(&route53.ListHostedZonesInput{}, aggr)
	if err != nil {
		return nil, err
	}

	zones := provider.DNSHostedZones{}
	for _, z := range raw {
		domain:=aws.StringValue(z.Name)
		comp := strings.Split(aws.StringValue(z.Id), "/")
		id := comp[len(comp)-1]
		forwarded := []string{}
		aggr := func(r *route53.ResourceRecordSet) {
			if aws.StringValue(r.Type) == dns.RS_NS {
				name := aws.StringValue(r.Name)
				if name != domain {
					forwarded = append(forwarded, dns.NormalizeHostname(name))
				}
			}
		}
		this.handleRecordSets(id, aggr)

		hostedZone := provider.NewDNSHostedZone(
			id, dns.NormalizeHostname(domain), aws.StringValue(z.Id), forwarded)
		zones = append(zones, hostedZone)
	}
	return zones, nil
}

func (this *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	aggr := func(r *route53.ResourceRecordSet) {
		rtype := aws.StringValue(r.Type)
		if dns.SupportedRecordType(rtype) {
			rs := dns.NewRecordSet(rtype, aws.Int64Value(r.TTL), nil)
			for _, rr := range r.ResourceRecords {
				rs.Add(&dns.Record{Value: aws.StringValue(rr.Value)})
			}
			dnssets.AddRecordSetFromProvider(aws.StringValue(r.Name), rs)
		}
	}
	if err := this.handleRecordSets(zone.Id(), aggr); err != nil {
		return nil, err
	}
	return provider.NewDNSZoneState(dnssets), nil
}

func (this *Handler) handleRecordSets(zoneid string, f func(rs *route53.ResourceRecordSet)) error {
	inp := (&route53.ListResourceRecordSetsInput{}).SetHostedZoneId(zoneid)
	aggr := func(resp *route53.ListResourceRecordSetsOutput, lastPage bool) (shouldContinue bool) {
		for _, r := range resp.ResourceRecordSets {
			f(r)
		}
		return true
	}

	return this.r53.ListResourceRecordSetsPages(inp, aggr)
}

func (this *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, this, zone)

	for _, r := range reqs {
		switch r.Action {
		case provider.R_CREATE:
			exec.addChange(route53.ChangeActionCreate, r, r.Addition)
		case provider.R_UPDATE:
			exec.addChange(route53.ChangeActionUpsert, r, r.Addition)
		case provider.R_DELETE:
			exec.addChange(route53.ChangeActionDelete, r, r.Deletion)
		}
	}
	if this.config.DryRun {
		logger.Infof("no changes in dryrun mode for AWS")
		return nil
	}
	return exec.submitChanges()
}
