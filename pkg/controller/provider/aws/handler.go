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
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	dnserrors "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type Handler struct {
	provider.DefaultDNSHandler
	config        provider.DNSHandlerConfig
	awsConfig     AWSConfig
	cache         provider.ZoneCache
	sess          *session.Session
	r53           *route53.Route53
	policyContext *routingPolicyContext
}

type AWSConfig struct {
	BatchSize int `json:"batchSize"`
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	advancedConfig := c.Options.AdvancedOptions.GetAdvancedConfig()
	c.Logger.Infof("advanced options: %s", advancedConfig)

	awsConfig := AWSConfig{BatchSize: advancedConfig.BatchSize}
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

	var creds *credentials.Credentials
	useCredentialsChain, err := c.GetDefaultedBoolProperty("AWS_USE_CREDENTIALS_CHAIN", false)
	if err != nil {
		return nil, fmt.Errorf("invalid value for AWS_USE_CREDENTIALS_CHAIN: %s", err)
	}
	if !useCredentialsChain {
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
		creds = credentials.NewStaticCredentials(accessKeyID, secretAccessKey, token)
	} else {
		if c.GetProperty("AWS_ACCESS_KEY_ID", "accessKeyID") != "" {
			return nil, fmt.Errorf("explicit credentials (AWS_ACCESS_KEY_ID or accessKeyID) cannot be used together with AWS_USE_CREDENTIALS_CHAIN=true")
		}
		c.Logger.Infof("creating aws-route53 handler using the chain of credential providers")
	}

	region := c.GetProperty("AWS_REGION", "region")
	var endpoint *string
	if region == "" {
		region = "us-west-2"
	}
	if strings.HasPrefix(region, "us-gov-") {
		endpoint = aws.String("route53.us-gov.amazonaws.com")
	}

	// change maxRetries to avoid paging stops because of throttling
	maxRetries := advancedConfig.MaxRetries
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: creds,
		Endpoint:    endpoint, // temporary workaround for AWS problem
		MaxRetries:  &maxRetries,
	})
	if err != nil {
		return nil, err
	}
	h.sess = sess
	h.r53 = route53.New(sess)
	h.policyContext = newRoutingPolicyContext(h.r53)

	h.cache, err = c.ZoneCacheFactory.CreateZoneCache(provider.CacheZoneState, c.Metrics, h.getZones, h.getZoneState)
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

func (h *Handler) getZones(_ provider.ZoneCache) (provider.DNSHostedZones, error) {
	blockedZones := h.config.Options.AdvancedOptions.GetBlockedZones()

	rt := provider.M_LISTZONES
	var raw []*route53.HostedZone
	aggr := func(resp *route53.ListHostedZonesOutput, lastPage bool) bool {
		for _, zone := range resp.HostedZones {
			comp := strings.Split(aws.StringValue(zone.Id), "/")
			id := comp[len(comp)-1]
			if blockedZones.Contains(id) {
				h.config.Logger.Infof("ignoring blocked zone id: %s", id)
				continue
			}
			raw = append(raw, zone)
		}
		h.config.Metrics.AddGenericRequests(rt, 1)
		rt = provider.M_PLISTZONES
		return true
	}

	h.config.RateLimiter.Accept()
	err := h.r53.ListHostedZonesPages(&route53.ListHostedZonesInput{}, aggr)
	if err != nil {
		return nil, err
	}

	zones := provider.DNSHostedZones{}
	for _, z := range raw {
		domain := aws.StringValue(z.Name)
		comp := strings.Split(aws.StringValue(z.Id), "/")
		id := comp[len(comp)-1]
		var isPrivateZone bool
		if z.Config.PrivateZone != nil && *z.Config.PrivateZone {
			isPrivateZone = true
		}
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), id, dns.NormalizeHostname(domain), aws.StringValue(z.Id), isPrivateZone)
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

func (h *Handler) getZoneState(zone provider.DNSHostedZone, _ provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	aggr := func(r *route53.ResourceRecordSet) {
		if dns.SupportedRecordType(aws.StringValue(r.Type)) {
			var rs *dns.RecordSet
			if isAliasTarget(r) {
				rs = buildRecordSetFromAliasTarget(r)
			} else {
				rs = buildRecordSet(r)
			}
			name := dns.DNSSetName{DNSName: aws.StringValue(r.Name), SetIdentifier: aws.StringValue(r.SetIdentifier)}
			policy := h.policyContext.extractRoutingPolicy(r)
			dnssets.AddRecordSetFromProviderEx(name, policy, rs)
		}
	}
	if err := h.handleRecordSets(zone, aggr); err != nil {
		var awsErr awserr.Error
		if errors.As(err, &awsErr) && awsErr.Code() == "NoSuchHostedZone" {
			err = &dnserrors.NoSuchHostedZone{ZoneId: zone.Id().ID, Err: err}
		}
		return nil, err
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) handleRecordSets(zone provider.DNSHostedZone, f func(rs *route53.ResourceRecordSet)) error {
	rt := provider.M_LISTRECORDS
	inp := (&route53.ListResourceRecordSetsInput{MaxItems: aws.String("300")}).SetHostedZoneId(zone.Id().ID)
	aggr := func(resp *route53.ListResourceRecordSetsOutput, lastPage bool) (shouldContinue bool) {
		h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
		for _, r := range resp.ResourceRecordSets {
			f(r)
		}
		rt = provider.M_PLISTRECORDS
		if !lastPage {
			h.config.RateLimiter.Accept()
		}

		return true
	}
	h.config.RateLimiter.Accept()
	return h.r53.ListResourceRecordSetsPages(inp, aggr)
}

func (h *Handler) ReportZoneStateConflict(zone provider.DNSHostedZone, err error) bool {
	return h.cache.ReportZoneStateConflict(zone, err)
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, _ provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h, zone)

	for _, r := range reqs {
		var err error
		switch r.Action {
		case provider.R_CREATE:
			err = exec.addChange(route53.ChangeActionCreate, r, r.Addition)
		case provider.R_UPDATE:
			err = exec.addChange(route53.ChangeActionUpsert, r, r.Addition)
		case provider.R_DELETE:
			err = exec.addChange(route53.ChangeActionDelete, r, r.Deletion)
		}
		if err != nil {
			if r.Done != nil {
				r.Done.SetInvalid(err)
			}
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
			return dnsutils.NewTarget(dns.RS_ALIAS, t.GetHostName(), t.GetTTL())
		}
	}
	return t
}

// AssociateVPCWithHostedZone associates a VPC with a private hosted zone
// in use by external controller
func (h *Handler) AssociateVPCWithHostedZone(vpcId string, vpcRegion string, hostedZoneId string) (*route53.AssociateVPCWithHostedZoneOutput, error) {
	input := route53.AssociateVPCWithHostedZoneInput{
		HostedZoneId: &hostedZoneId,
		VPC:          &route53.VPC{VPCId: &vpcId, VPCRegion: &vpcRegion},
	}
	h.config.RateLimiter.Accept()
	out, err := h.r53.AssociateVPCWithHostedZone(&input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DisassociateVPCFromHostedZone disassociates a VPC from a private hosted zone
// in use by external controller
func (h *Handler) DisassociateVPCFromHostedZone(vpcId string, vpcRegion string, hostedZoneId string) (*route53.DisassociateVPCFromHostedZoneOutput, error) {
	input := route53.DisassociateVPCFromHostedZoneInput{
		HostedZoneId: &hostedZoneId,
		VPC:          &route53.VPC{VPCId: &vpcId, VPCRegion: &vpcRegion},
	}
	h.config.RateLimiter.Accept()
	out, err := h.r53.DisassociateVPCFromHostedZone(&input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetZoneByName returns detailed information about a zone
// in use by external controller
func (h *Handler) GetZoneByName(hostedZoneId string) (*route53.GetHostedZoneOutput, error) {
	input := route53.GetHostedZoneInput{
		Id: &hostedZoneId,
	}
	h.config.RateLimiter.Accept()
	out, err := h.r53.GetHostedZone(&input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreateVPCAssociationAuthorization authorizes the AWS account that created a specified VPC to submit an AssociateVPCWithHostedZone
// request to associate the VPC with a specified hosted zone that was created
// by a different account
func (h *Handler) CreateVPCAssociationAuthorization(hostedZoneId string, vpcId string, vpcRegion string) (*route53.CreateVPCAssociationAuthorizationOutput, error) {
	input := route53.CreateVPCAssociationAuthorizationInput{
		HostedZoneId: &hostedZoneId,
		VPC: &route53.VPC{
			VPCId:     &vpcId,
			VPCRegion: &vpcRegion,
		},
	}
	h.config.RateLimiter.Accept()
	out, err := h.r53.CreateVPCAssociationAuthorization(&input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteVPCAssociationAuthorization removes authorization to submit an AssociateVPCWithHostedZone request to
// associate a specified VPC with a hosted zone that was created by a different account.
func (h *Handler) DeleteVPCAssociationAuthorization(hostedZoneId string, vpcId string, vpcRegion string) (*route53.DeleteVPCAssociationAuthorizationOutput, error) {
	input := route53.DeleteVPCAssociationAuthorizationInput{
		HostedZoneId: &hostedZoneId,
		VPC: &route53.VPC{
			VPCId:     &vpcId,
			VPCRegion: &vpcRegion,
		},
	}
	h.config.RateLimiter.Accept()
	out, err := h.r53.DeleteVPCAssociationAuthorization(&input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *Handler) GetRecordSet(zone provider.DNSHostedZone, setName dns.DNSSetName, recordType string) (provider.DedicatedRecordSet, error) {
	name := setName.Align()
	var recordIdentifier *string
	if setName.SetIdentifier != "" {
		recordIdentifier = &setName.SetIdentifier
	}
	sets, err := h.r53.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId:          aws.String(zone.Id().ID),
		MaxItems:              aws.String("1"),
		StartRecordIdentifier: recordIdentifier,
		StartRecordName:       &name.DNSName,
		StartRecordType:       &recordType,
	})
	if err != nil {
		return nil, err
	}

	dnssets := dns.DNSSets{}
	aggr := func(r *route53.ResourceRecordSet) {
		if dns.SupportedRecordType(aws.StringValue(r.Type)) {
			var rs *dns.RecordSet
			if isAliasTarget(r) {
				rs = buildRecordSetFromAliasTarget(r)
			} else {
				rs = buildRecordSet(r)
			}
			routingPolicy := h.policyContext.extractRoutingPolicy(r)
			dnsSetName := dns.DNSSetName{DNSName: aws.StringValue(r.Name), SetIdentifier: aws.StringValue(r.SetIdentifier)}
			dnssets.AddRecordSetFromProviderEx(dnsSetName, routingPolicy, rs)
		}
	}
	for _, r := range sets.ResourceRecordSets {
		if aws.StringValue(r.Name) == name.DNSName && aws.StringValue(r.SetIdentifier) == name.SetIdentifier && aws.StringValue(r.Type) == recordType {
			aggr(r)
		}
	}
	if set := dnssets[setName]; set != nil {
		return provider.FromDedicatedRecordSet(setName, set.Sets[recordType]), nil
	}
	return nil, nil
}

func (h *Handler) CreateOrUpdateRecordSet(logger logger.LogContext, zone provider.DNSHostedZone, _, new provider.DedicatedRecordSet) error {
	return h.executeRecordSetChange(route53.ChangeActionUpsert, logger, zone, new)
}

func (h *Handler) DeleteRecordSet(logger logger.LogContext, zone provider.DNSHostedZone, rs provider.DedicatedRecordSet) error {
	return h.executeRecordSetChange(route53.ChangeActionDelete, logger, zone, rs)
}

func (h *Handler) executeRecordSetChange(action string, logger logger.LogContext, zone provider.DNSHostedZone, rawrs provider.DedicatedRecordSet) error {
	exec := NewExecution(logger, h, zone)
	dnsName, rs := provider.ToDedicatedRecordset(rawrs)
	dnsset := dns.NewDNSSet(dnsName, nil)
	dnsset.Sets[rs.Type] = rs
	if err := exec.addChange(action, &provider.ChangeRequest{Type: rs.Type}, dnsset); err != nil {
		return err
	}
	return exec.submitChanges(h.config.Metrics)
}
