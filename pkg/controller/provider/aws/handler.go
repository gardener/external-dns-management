// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	v2config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

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
	r53           route53.Client
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

	region := c.GetProperty("AWS_REGION", "region")
	if region == "" {
		region = "us-west-2"
	}

	var awscfg aws.Config
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
		awscfg, err = v2config.LoadDefaultConfig(
			context.TODO(),
			v2config.WithRegion(region),
			v2config.WithAppID("gardener-external-dns-management"),
			v2config.WithCredentialsProvider(aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, token))),
			v2config.WithRetryMaxAttempts(advancedConfig.MaxRetries), // change maxRetries to avoid paging stops because of throttling
		)
		if err != nil {
			return nil, err
		}
	} else {
		if c.GetProperty("AWS_ACCESS_KEY_ID", "accessKeyID") != "" {
			return nil, fmt.Errorf("explicit credentials (AWS_ACCESS_KEY_ID or accessKeyID) cannot be used together with AWS_USE_CREDENTIALS_CHAIN=true")
		}
		c.Logger.Infof("creating aws-route53 handler using the chain of credential providers")
	}

	// TODO check if this is correct
	//if strings.HasPrefix(region, "us-gov-") {
	//	awscfg.BaseEndpoint = aws.String("route53.us-gov.amazonaws.com")
	//}

	h.r53 = *route53.NewFromConfig(awscfg)
	if err != nil {
		return nil, err
	}
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
	var zones provider.DNSHostedZones

	h.config.RateLimiter.Accept()
	ctx := context.Background()
	paginator := route53.NewListHostedZonesPaginator(&h.r53, &route53.ListHostedZonesInput{})
	for paginator.HasMorePages() {
		h.config.Metrics.AddGenericRequests(rt, 1)
		rt = provider.M_PLISTZONES

		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, zone := range output.HostedZones {
			comp := strings.Split(aws.ToString(zone.Id), "/")
			id := comp[len(comp)-1]
			if blockedZones.Contains(id) {
				h.config.Logger.Infof("ignoring blocked zone id: %s", id)
				continue
			}

			domain := dns.NormalizeHostname(aws.ToString(zone.Name))
			hostedZone := provider.NewDNSHostedZone(h.ProviderType(), id, domain, aws.ToString(zone.Id), zone.Config.PrivateZone)
			zones = append(zones, hostedZone)
		}
	}

	return zones, nil
}

func buildRecordSet(r route53types.ResourceRecordSet) *dns.RecordSet {
	if rs := buildRecordSetFromAliasTarget(r); rs != nil {
		return rs
	}
	rs := dns.NewRecordSet(string(r.Type), aws.ToInt64(r.TTL), nil)
	for _, rr := range r.ResourceRecords {
		rs.Add(&dns.Record{Value: aws.ToString(rr.Value)})
	}
	return rs
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, _ provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}
	ctx := context.Background()

	rt := provider.M_LISTRECORDS
	input := &route53.ListResourceRecordSetsInput{MaxItems: aws.Int32(300), HostedZoneId: aws.String(zone.Id().ID)}
	paginator := route53.NewListResourceRecordSetsPaginator(&h.r53, input)
	for paginator.HasMorePages() {
		h.config.RateLimiter.Accept()
		h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
		rt = provider.M_PLISTRECORDS

		output, err := paginator.NextPage(ctx)
		if err != nil {
			var target *route53types.NoSuchHostedZone
			if errors.As(err, &target) {
				err = &dnserrors.NoSuchHostedZone{ZoneId: zone.Id().ID, Err: err}
			}
			return nil, err
		}
		for _, r := range output.ResourceRecordSets {
			if dns.SupportedRecordType(string(r.Type)) {
				rs := buildRecordSet(r)
				name := dns.DNSSetName{DNSName: aws.ToString(r.Name), SetIdentifier: aws.ToString(r.SetIdentifier)}
				policy := h.policyContext.extractRoutingPolicy(ctx, &r)
				dnssets.AddRecordSetFromProviderEx(name, policy, rs)
			}
		}
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ReportZoneStateConflict(zone provider.DNSHostedZone, err error) bool {
	return h.cache.ReportZoneStateConflict(zone, err)
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	ctx := context.Background()
	err := h.executeRequests(ctx, logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(ctx context.Context, logger logger.LogContext, zone provider.DNSHostedZone, _ provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h, zone)

	for _, r := range reqs {
		var err error
		switch r.Action {
		case provider.R_CREATE:
			err = exec.addChange(ctx, route53types.ChangeActionCreate, r, r.Addition)
		case provider.R_UPDATE:
			err = exec.addChange(ctx, route53types.ChangeActionUpsert, r, r.Addition)
		case provider.R_DELETE:
			err = exec.addChange(ctx, route53types.ChangeActionDelete, r, r.Deletion)
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
	return exec.submitChanges(ctx, h.config.Metrics)
}

func (h *Handler) MapTargets(_ string, targets []provider.Target) []provider.Target {
	return MapTargets(targets)
}

// MapTargets maps CNAME records to A/AAAA records for hosted zones used for AWS load balancers.
func MapTargets(targets []provider.Target) []provider.Target {
	mapped := make([]provider.Target, 0, len(targets)+1)
	for _, t := range targets {
		switch t.GetRecordType() {
		case dns.RS_CNAME:
			hostedZone := canonicalHostedZone(t.GetHostName())
			if hostedZone != "" {
				switch strings.ToLower(t.GetIPStack()) {
				case dns.AnnotationValueIPStackIPDualStack:
					mapped = append(mapped, dnsutils.NewTarget(dns.RS_ALIAS_A, t.GetHostName(), t.GetTTL()))
					mapped = append(mapped, dnsutils.NewTarget(dns.RS_ALIAS_AAAA, t.GetHostName(), t.GetTTL()))
				case dns.AnnotationValueIPStackIPv6:
					mapped = append(mapped, dnsutils.NewTarget(dns.RS_ALIAS_AAAA, t.GetHostName(), t.GetTTL()))
				default:
					mapped = append(mapped, dnsutils.NewTarget(dns.RS_ALIAS_A, t.GetHostName(), t.GetTTL()))
				}
			} else {
				mapped = append(mapped, t)
			}
		default:
			mapped = append(mapped, t)
		}
	}
	return mapped
}

// AssociateVPCWithHostedZone associates a VPC with a private hosted zone
// in use by external controller
func (h *Handler) AssociateVPCWithHostedZone(ctx context.Context, vpcId string, vpcRegion route53types.VPCRegion, hostedZoneId string) (*route53.AssociateVPCWithHostedZoneOutput, error) {
	input := &route53.AssociateVPCWithHostedZoneInput{
		HostedZoneId: &hostedZoneId,
		VPC:          &route53types.VPC{VPCId: &vpcId, VPCRegion: vpcRegion},
	}
	h.config.RateLimiter.Accept()
	out, err := h.r53.AssociateVPCWithHostedZone(ctx, input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DisassociateVPCFromHostedZone disassociates a VPC from a private hosted zone
// in use by external controller
func (h *Handler) DisassociateVPCFromHostedZone(ctx context.Context, vpcId string, vpcRegion route53types.VPCRegion, hostedZoneId string) (*route53.DisassociateVPCFromHostedZoneOutput, error) {
	input := &route53.DisassociateVPCFromHostedZoneInput{
		HostedZoneId: &hostedZoneId,
		VPC:          &route53types.VPC{VPCId: &vpcId, VPCRegion: vpcRegion},
	}
	h.config.RateLimiter.Accept()
	out, err := h.r53.DisassociateVPCFromHostedZone(ctx, input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetZoneByName returns detailed information about a zone
// in use by external controller
func (h *Handler) GetZoneByName(hostedZoneId string) (*route53.GetHostedZoneOutput, error) {
	input := &route53.GetHostedZoneInput{
		Id: &hostedZoneId,
	}
	h.config.RateLimiter.Accept()
	ctx := context.Background()
	out, err := h.r53.GetHostedZone(ctx, input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreateVPCAssociationAuthorization authorizes the AWS account that created a specified VPC to submit an AssociateVPCWithHostedZone
// request to associate the VPC with a specified hosted zone that was created
// by a different account
func (h *Handler) CreateVPCAssociationAuthorization(ctx context.Context, hostedZoneId string, vpcId string, vpcRegion route53types.VPCRegion) (*route53.CreateVPCAssociationAuthorizationOutput, error) {
	input := &route53.CreateVPCAssociationAuthorizationInput{
		HostedZoneId: &hostedZoneId,
		VPC: &route53types.VPC{
			VPCId:     &vpcId,
			VPCRegion: vpcRegion,
		},
	}
	h.config.RateLimiter.Accept()
	out, err := h.r53.CreateVPCAssociationAuthorization(ctx, input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteVPCAssociationAuthorization removes authorization to submit an AssociateVPCWithHostedZone request to
// associate a specified VPC with a hosted zone that was created by a different account.
func (h *Handler) DeleteVPCAssociationAuthorization(ctx context.Context, hostedZoneId string, vpcId string, vpcRegion route53types.VPCRegion) (*route53.DeleteVPCAssociationAuthorizationOutput, error) {
	input := &route53.DeleteVPCAssociationAuthorizationInput{
		HostedZoneId: &hostedZoneId,
		VPC: &route53types.VPC{
			VPCId:     &vpcId,
			VPCRegion: vpcRegion,
		},
	}
	h.config.RateLimiter.Accept()
	out, err := h.r53.DeleteVPCAssociationAuthorization(ctx, input)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *Handler) GetRecordSet(ctx context.Context, zone provider.DNSHostedZone, setName dns.DNSSetName, recordType route53types.RRType) (provider.DedicatedRecordSet, error) {
	name := setName.Align()
	var recordIdentifier *string
	if setName.SetIdentifier != "" {
		recordIdentifier = &setName.SetIdentifier
	}
	sets, err := h.r53.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:          aws.String(zone.Id().ID),
		MaxItems:              aws.Int32(1),
		StartRecordIdentifier: recordIdentifier,
		StartRecordName:       &name.DNSName,
		StartRecordType:       recordType,
	})
	if err != nil {
		return nil, err
	}

	dnssets := dns.DNSSets{}
	aggr := func(r route53types.ResourceRecordSet) {
		if dns.SupportedRecordType(string(r.Type)) {
			rs := buildRecordSet(r)
			routingPolicy := h.policyContext.extractRoutingPolicy(ctx, &r)
			dnsSetName := dns.DNSSetName{DNSName: aws.ToString(r.Name), SetIdentifier: aws.ToString(r.SetIdentifier)}
			dnssets.AddRecordSetFromProviderEx(dnsSetName, routingPolicy, rs)
		}
	}
	for _, r := range sets.ResourceRecordSets {
		if aws.ToString(r.Name) == name.DNSName && aws.ToString(r.SetIdentifier) == name.SetIdentifier && r.Type == recordType {
			aggr(r)
		}
	}
	if set := dnssets[setName]; set != nil {
		return provider.FromDedicatedRecordSet(setName, set.Sets[string(recordType)]), nil
	}
	return nil, nil
}

func (h *Handler) CreateOrUpdateRecordSet(ctx context.Context, logger logger.LogContext, zone provider.DNSHostedZone, _, new provider.DedicatedRecordSet) error {
	return h.executeRecordSetChange(ctx, route53types.ChangeActionUpsert, logger, zone, new)
}

func (h *Handler) DeleteRecordSet(ctx context.Context, logger logger.LogContext, zone provider.DNSHostedZone, rs provider.DedicatedRecordSet) error {
	return h.executeRecordSetChange(ctx, route53types.ChangeActionDelete, logger, zone, rs)
}

func (h *Handler) executeRecordSetChange(ctx context.Context, action route53types.ChangeAction, logger logger.LogContext, zone provider.DNSHostedZone, rawrs provider.DedicatedRecordSet) error {
	exec := NewExecution(logger, h, zone)
	dnsName, rs := provider.ToDedicatedRecordset(rawrs)
	dnsset := dns.NewDNSSet(dnsName, nil)
	dnsset.Sets[rs.Type] = rs
	if err := exec.addChange(ctx, action, &provider.ChangeRequest{Type: rs.Type}, dnsset); err != nil {
		return err
	}
	return exec.submitChanges(ctx, h.config.Metrics)
}
