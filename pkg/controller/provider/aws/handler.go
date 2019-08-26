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
	"github.com/aws/aws-sdk-go/aws/awserr"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns/provider"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/gardener/external-dns-management/pkg/dns"
)

type Handler struct {
	provider.DefaultDNSHandler
	config    provider.DNSHandlerConfig
	awsConfig AWSConfig
	cache     provider.ZoneCache
	sess      *session.Session
	r53       *route53.Route53
}

type AWSConfig struct {
	BatchSize int `json:"batchSize"`
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	awsConfig := AWSConfig{BatchSize: 50}
	if c.Config != nil {
		err := json.Unmarshal(c.Config.Raw, &awsConfig)
		if err != nil {
			return nil, fmt.Errorf("unmarshal aws-route providerConfig failed with: %s", err)
		}
	}

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *c,
		awsConfig:         awsConfig,
	}
	accessKeyID, err := c.GetRequiredProperty("AWS_ACCESS_KEY_ID", "accessKeyID")
	if err != nil {
		return nil, err
	}
	c.Logger.Infof("creating aws-route53 handler for %s", accessKeyID)
	secretAccessKey, err := c.GetRequiredProperty("AWS_SECRET_ACCESS_KEY", "secretAccessKey")
	if err != nil {
		return nil, err
	}
	token := c.GetProperty("AWS_SESSION_TOKEN")
	creds := credentials.NewStaticCredentials(accessKeyID, secretAccessKey, token)

	region := c.GetProperty("AWS_REGION", "region")
	var endpoint *string
	if region == "" {
		region = "us-west-2"
	}
	if strings.HasPrefix(region, "us-gov-") {
		endpoint = aws.String("route53.us-gov.amazonaws.com")
	}

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: creds,
		Endpoint:    endpoint, // temporary workarround for AWS problem
	})
	if err != nil {
		return nil, err
	}
	h.sess = sess
	h.r53 = route53.New(sess)

	forwardedDomains := provider.NewForwardedDomainsHandlerData()
	h.cache, err = provider.NewZoneCache(c.CacheConfig, c.Metrics, forwardedDomains, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (h *Handler) Release() {
	h.cache.Release()
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) getZones(cache provider.ZoneCache) (provider.DNSHostedZones, error) {
	rt := provider.M_LISTZONES
	raw := []*route53.HostedZone{}
	aggr := func(resp *route53.ListHostedZonesOutput, lastPage bool) bool {
		for _, zone := range resp.HostedZones {
			raw = append(raw, zone)
		}
		h.config.Metrics.AddRequests(rt, 1)
		rt = provider.M_PLISTZONES
		return true
	}

	err := h.r53.ListHostedZonesPages(&route53.ListHostedZonesInput{}, aggr)
	if err != nil {
		return nil, err
	}

	zones := provider.DNSHostedZones{}
	for _, z := range raw {
		domain := aws.StringValue(z.Name)
		comp := strings.Split(aws.StringValue(z.Id), "/")
		id := comp[len(comp)-1]
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(),
			id, dns.NormalizeHostname(domain), aws.StringValue(z.Id), []string{})

		// call GetZoneState for side effect to calculate forwarded domains
		_, err := cache.GetZoneState(hostedZone)
		if err == nil {
			forwarded := cache.GetHandlerData().(*provider.ForwardedDomainsHandlerData).GetForwardedDomains(hostedZone.Id())
			if forwarded != nil {
				hostedZone = provider.CopyDNSHostedZone(hostedZone, forwarded)
			}
		} else {
			a, ok := err.(awserr.Error)
			if ok {
				if a.Code() == "AccessDenied" {
					h.config.Logger.Warnf("AWS permission missing for zone %s -> omit zone: %s", aws.StringValue(z.Id), err)
					continue
				} else {
					h.config.Logger.Warnf("AWS error during get zone state for %s: %s: %s", aws.StringValue(z.Id), a.Code(), err)
				}
			} else {
				h.config.Logger.Warnf("Error during get zone state for %s: %s", aws.StringValue(z.Id), err)
			}
		}

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

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	aggr := func(r *route53.ResourceRecordSet) {
		if dns.SupportedRecordType(aws.StringValue(r.Type)) {
			var rs *dns.RecordSet
			if isAliasTarget(r) {
				rs = buildRecordSetFromAliasTarget(r)
			} else {
				rs = buildRecordSet(r)
			}
			dnssets.AddRecordSetFromProvider(aws.StringValue(r.Name), rs)
		}
	}
	forwarded, err := h.handleRecordSets(zone, aggr)
	if err != nil {
		return nil, err
	}

	cache.GetHandlerData().(*provider.ForwardedDomainsHandlerData).SetForwardedDomains(zone.Id(), forwarded)

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) handleRecordSets(zone provider.DNSHostedZone, f func(rs *route53.ResourceRecordSet)) ([]string, error) {
	rt := provider.M_LISTRECORDS
	inp := (&route53.ListResourceRecordSetsInput{}).SetHostedZoneId(zone.Id())
	forwarded := []string{}
	aggr := func(resp *route53.ListResourceRecordSetsOutput, lastPage bool) (shouldContinue bool) {
		h.config.Metrics.AddRequests(rt, 1)
		for _, r := range resp.ResourceRecordSets {
			f(r)
			if aws.StringValue(r.Type) == dns.RS_NS {
				name := dns.NormalizeHostname(aws.StringValue(r.Name))
				if name != zone.Domain() {
					forwarded = append(forwarded, name)
				}
			}
		}
		rt = provider.M_PLISTRECORDS

		return true
	}
	err := h.r53.ListResourceRecordSetsPages(inp, aggr)
	return forwarded, err
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h, zone)

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
	if h.config.DryRun {
		logger.Infof("no changes in dryrun mode for AWS")
		return nil
	}
	return exec.submitChanges(h.config.Metrics)
}

func (h *Handler) MapTarget(t provider.Target) provider.Target {
	if t.GetRecordType() == dns.RS_CNAME {
		hostedZone := canonicalHostedZone(t.GetHostName())
		if hostedZone != "" {
			return provider.NewTarget(dns.RS_ALIAS, t.GetHostName(), t.GetEntry())
		}
	}
	return t
}
