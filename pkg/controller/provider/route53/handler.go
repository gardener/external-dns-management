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
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/gardener/external-dns-management/pkg/dns"
)

type Handler struct {
	config dns.DNSHandlerConfig

	sess *session.Session
	r53  *route53.Route53
}

var _ dns.DNSHandler = &Handler{}

func NewHandler(logger logger.LogContext, config *dns.DNSHandlerConfig) (dns.DNSHandler, error) {
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

func (this *Handler) GetZones() (dns.DNSHostedZoneInfos, error) {
	zones := []*dns.DNSHostedZoneInfo{}

	aggr := func(resp *route53.ListHostedZonesOutput, lastPage bool) bool {
		for _, zone := range resp.HostedZones {
			id := strings.Split(aws.StringValue(zone.Id), "/")

			zoneinfo := &dns.DNSHostedZoneInfo{
				Id:     id[len(id)-1],
				Domain: dns.NormalizeHostname(aws.StringValue(zone.Name)),
			}
			zones = append(zones, zoneinfo)
		}
		return true
	}

	err := this.r53.ListHostedZonesPages(&route53.ListHostedZonesInput{}, aggr)
	if err != nil {
		return nil, err
	}
	return zones, nil
}

func (this *Handler) GetDNSSets(zoneid string) (dns.DNSSets, error) {
	dnssets := dns.DNSSets{}

	inp := (&route53.ListResourceRecordSetsInput{}).SetHostedZoneId(zoneid)
	aggr := func(resp *route53.ListResourceRecordSetsOutput, lastPage bool) (shouldContinue bool) {
		for _, r := range resp.ResourceRecordSets {
			rtype := aws.StringValue(r.Type)
			if !dns.SupportedRecordType(rtype) {
				continue
			}

			rs := dns.NewRecordSet(rtype, aws.Int64Value(r.TTL), nil)
			for _, rr := range r.ResourceRecords {
				rs.Add(&dns.Record{Value: aws.StringValue(rr.Value)})
			}

			dnssets.AddRecordSetFromProvider(aws.StringValue(r.Name), rs)
		}
		return true
	}

	if err := this.r53.ListResourceRecordSetsPages(inp, aggr); err != nil {
		return nil, err
	}
	return dnssets, nil
}

func (this *Handler) ExecuteRequests(logger logger.LogContext, zoneid string, reqs []*dns.ChangeRequest) error {
	exec := NewExecution(logger, this, zoneid)

	for _, r := range reqs {
		switch r.Action {
		case dns.R_CREATE:
			exec.addChange(route53.ChangeActionCreate, r, r.Addition)
		case dns.R_UPDATE:
			exec.addChange(route53.ChangeActionUpsert, r, r.Addition)
		case dns.R_DELETE:
			exec.addChange(route53.ChangeActionDelete, r, r.Deletion)
		}
	}
	if this.config.DryRun {
		logger.Infof("no changes in dryrun mode for AWS")
		return nil
	}
	return exec.submitChanges()
}
