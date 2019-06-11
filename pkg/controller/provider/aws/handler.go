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
	"encoding/json"
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
	config    provider.DNSHandlerConfig
	awsConfig AWSConfig
	metrics   provider.Metrics
	sess      *session.Session
	r53       *route53.Route53
}

type AWSConfig struct {
	BatchSize int `json:"batchSize"`
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(logger logger.LogContext, config *provider.DNSHandlerConfig, metrics provider.Metrics) (provider.DNSHandler, error) {
	awsConfig := AWSConfig{BatchSize: 50}
	if config.Config != nil {
		err := json.Unmarshal(config.Config.Raw, &awsConfig)
		if err != nil {
			return nil, fmt.Errorf("unmarshal aws-route providerConfig failed with: %s", err)
		}
	}

	this := &Handler{
		config:    *config,
		awsConfig: awsConfig,
		metrics:   metrics,
	}
	akid := this.config.Properties["AWS_ACCESS_KEY_ID"]
	if akid == "" {
		logger.Infof("creating aws-route53 handler failed because of missing access key id")
		return nil, fmt.Errorf("'AWS_ACCESS_KEY_ID' required in secret")
	}
	logger.Infof("creating aws-route53 handler for %s", akid)
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

func (h *Handler) ProviderType() string {
	return TYPE_CODE
}

func (this *Handler) GetZones() (provider.DNSHostedZones, error) {
	rt := provider.M_LISTZONES
	raw := []*route53.HostedZone{}
	aggr := func(resp *route53.ListHostedZonesOutput, lastPage bool) bool {
		for _, zone := range resp.HostedZones {
			raw = append(raw, zone)
		}
		this.metrics.AddRequests(rt, 1)
		rt = provider.M_PLISTZONES
		return true
	}

	err := this.r53.ListHostedZonesPages(&route53.ListHostedZonesInput{}, aggr)
	if err != nil {
		return nil, err
	}

	zones := provider.DNSHostedZones{}
	for _, z := range raw {
		domain := aws.StringValue(z.Name)
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

		hostedZone := provider.NewDNSHostedZone(this.ProviderType(),
			id, dns.NormalizeHostname(domain), aws.StringValue(z.Id), forwarded)
		zones = append(zones, hostedZone)
	}
	return zones, nil
}

func buildRecordSet(r *route53.ResourceRecordSet) *dns.RecordSet {
	rs := dns.NewRecordSet(aws.StringValue(r.Type), aws.Int64Value(r.TTL), nil)
	for _, rr := range r.ResourceRecords {
		rs.Add(&dns.Record{Value: aws.StringValue(rr.Value)})
	}
	return rs
}

func (this *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	migrations := []*route53.ResourceRecordSet{}

	aggr := func(r *route53.ResourceRecordSet) {
		if dns.SupportedRecordType(aws.StringValue(r.Type)) {
			var rs *dns.RecordSet
			if isAliasTarget(r) {
				rs = buildRecordSetForAliasTarget(r)
			} else {
				rs = buildRecordSet(r)
				if canConvertToAliasTarget(rs) {
					migrations = append(migrations, r)
				}
			}
			dnssets.AddRecordSetFromProvider(aws.StringValue(r.Name), rs)
		}
	}
	if err := this.handleRecordSets(zone.Id(), aggr); err != nil {
		return nil, err
	}

	if len(migrations) > 0 {
		this.migrateRecordsToAliasTargets(zone, migrations)
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (this *Handler) handleRecordSets(zoneid string, f func(rs *route53.ResourceRecordSet)) error {
	rt := provider.M_LISTRECORDS
	inp := (&route53.ListResourceRecordSetsInput{}).SetHostedZoneId(zoneid)
	aggr := func(resp *route53.ListResourceRecordSetsOutput, lastPage bool) (shouldContinue bool) {
		this.metrics.AddRequests(rt, 1)
		for _, r := range resp.ResourceRecordSets {
			f(r)
		}
		rt = provider.M_PLISTRECORDS

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
	return exec.submitChanges(this.metrics)
}

func (this *Handler) migrateRecordsToAliasTargets(zone provider.DNSHostedZone, migrations []*route53.ResourceRecordSet) {
	logContext := logger.NewContext("provider", "aws-route53").NewContext("zone", zone.Id())
	logContext.Infof("migrating %d records to alias targets", len(migrations))
	exec := NewExecution(logContext, this, zone)

	for _, r := range migrations {
		rs := buildRecordSet(r)
		name := aws.StringValue(r.Name)
		dnsset := dns.NewDNSSet(dns.NormalizeHostname(name))
		dnsset.Sets[rs.Type] = rs

		// delete old CNAME DNS record
		change := &route53.Change{Action: aws.String(route53.ChangeActionDelete), ResourceRecordSet: r}
		exec.addRawChange(name, change, nil)
		// add A alias target record (implicitly converted dns.RecordSet)
		r := &provider.ChangeRequest{Action: provider.R_CREATE, Type: aws.StringValue(r.Type), Addition: dnsset}
		exec.addChange(route53.ChangeActionCreate, r, r.Addition)
	}

	err := exec.submitChanges(this.metrics)
	if err != nil {
		logContext.Warnf("Migrating to alias targets failed with %s", err)
	}
}
