// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	v2config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	workloadidentityaws "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/aws"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type handler struct {
	provider.DefaultDNSHandler
	config        provider.DNSHandlerConfig
	awsConfig     AWSConfig
	accessKeyID   string // for logging purposes
	r53           route53.Client
	policyContext *routingPolicyContext
}

var _ provider.DNSHandler = &handler{}

// AWSConfig holds the provider configuration for the AWS Route53 DNS handler.
type AWSConfig struct {
	BatchSize int `json:"batchSize"`
}

// NewHandler creates a new AWS Route53 DNS handler based on the provided configuration.
func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	advancedOptions := c.GlobalConfig.ProviderAdvancedOptions[ProviderType]
	c.Log.Info("advanced options", "options", advancedOptions) // TODO(MartinWeindel) fix logging of advanced options

	awsConfig := AWSConfig{BatchSize: ptr.Deref(advancedOptions.BatchSize, defaultBatchSize)}
	if c.Config != nil {
		err := json.Unmarshal(c.Config.Raw, &awsConfig)
		if err != nil {
			return nil, fmt.Errorf("unmarshal aws-route providerConfig failed with: %s", err)
		}
	}

	h := &handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *c,
		awsConfig:         awsConfig,
	}

	region := c.GetProperty("AWS_REGION", "region")
	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-west-2"
		}
	}
	c.Log.Info("using region", "region", region)

	var awscfg aws.Config
	useCredentialsChain, err := c.GetDefaultedBoolProperty("AWS_USE_CREDENTIALS_CHAIN", false)
	if err != nil {
		return nil, fmt.Errorf("invalid value for AWS_USE_CREDENTIALS_CHAIN: %s", err)
	}
	configOptions := []func(*v2config.LoadOptions) error{
		v2config.WithRegion(region),
		v2config.WithAppID("gardener-external-dns-management"),
		v2config.WithRetryMaxAttempts(ptr.Deref(advancedOptions.MaxRetries, defaultMaxRetries)), // change maxRetries to avoid paging stops because of throttling
	}
	switch {
	case c.GetProperty(securityv1alpha1constants.LabelWorkloadIdentityProvider) == "aws":
		dataKeyToken, err := c.GetRequiredProperty(securityv1alpha1constants.DataKeyToken)
		if err != nil {
			return nil, err
		}
		roleARN, err := getRoleARN(c)
		if err != nil {
			return nil, err
		}
		c.Log.Info("creating aws-route53 handler using web identity role", "roleARN", roleARN)
		credentialsProvider := stscreds.NewWebIdentityRoleProvider(
			sts.NewFromConfig(aws.Config{Region: region}),
			roleARN,
			&staticTokenRetriever{token: []byte(dataKeyToken)},
		)
		configOptions = append(configOptions, v2config.WithCredentialsProvider(credentialsProvider))
	case !useCredentialsChain:
		accessKeyID, err := c.GetRequiredProperty("AWS_ACCESS_KEY_ID", "accessKeyID")
		if err != nil {
			return nil, err
		}
		c.Log.Info("creating aws-route53 handler", "accessKeyID", accessKeyID)
		h.accessKeyID = accessKeyID // store for logging purposes
		secretAccessKey, err := c.GetRequiredProperty("AWS_SECRET_ACCESS_KEY", "secretAccessKey")
		if err != nil {
			return nil, err
		}
		token := c.GetProperty("AWS_SESSION_TOKEN", "sessionToken")
		configOptions = append(configOptions, v2config.WithCredentialsProvider(aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, token))))
	default:
		if c.GetProperty("AWS_ACCESS_KEY_ID", "accessKeyID") != "" {
			return nil, fmt.Errorf("explicit credentials (AWS_ACCESS_KEY_ID or accessKeyID) cannot be used together with AWS_USE_CREDENTIALS_CHAIN=true")
		}
		c.Log.Info("creating aws-route53 handler using the chain of credential providers")
	}

	awscfg, err = v2config.LoadDefaultConfig(context.Background(), configOptions...)
	if err != nil {
		return nil, err
	}

	h.r53 = *route53.NewFromConfig(awscfg)
	h.policyContext = newRoutingPolicyContext(h.r53)

	return h, nil
}

func (h *handler) Release() {
}

func (h *handler) getAdvancedOptions() config.AdvancedOptions {
	return h.config.GlobalConfig.ProviderAdvancedOptions[ProviderType]
}
func (h *handler) GetZones(ctx context.Context) ([]provider.DNSHostedZone, error) {
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return nil, err
	}

	blockedZones := h.getAdvancedOptions().BlockedZones

	rt := provider.MetricsRequestTypeListZones
	var zones []provider.DNSHostedZone

	h.config.RateLimiter.Accept()
	paginator := route53.NewListHostedZonesPaginator(&h.r53, &route53.ListHostedZonesInput{})
	for paginator.HasMorePages() {
		h.config.Metrics.AddGenericRequests(rt, 1)
		rt = provider.MetricsRequestTypeListZonesPages

		output, err := paginator.NextPage(ctx)
		if err != nil {
			log.Error(err, "listing hosted zones failed")
			return nil, stableError(err)
		}
	outer:
		for _, zone := range output.HostedZones {
			comp := strings.Split(aws.ToString(zone.Id), "/")
			id := comp[len(comp)-1]
			for _, zone := range blockedZones {
				if zone == id {
					log.Info("ignoring blocked zone", "zone", id)
					continue outer
				}
			}

			domain := dns.NormalizeDomainName(aws.ToString(zone.Name))
			hostedZone := provider.NewDNSHostedZone(h.ProviderType(), id, domain, aws.ToString(zone.Id), zone.Config.PrivateZone)
			zones = append(zones, hostedZone)
		}
	}

	return zones, nil
}

// GetCustomQueryDNSFunc returns a custom DNS query function for the AWS Route53 provider if the zone is private.
func (h *handler) GetCustomQueryDNSFunc(zone dns.ZoneInfo, factory utils.QueryDNSFactoryFunc) (provider.CustomQueryDNSFunc, error) {
	if zone.IsPrivate() {
		return h.queryDNS, nil
	}
	defaultQueryFunc, err := factory()
	if err != nil {
		return nil, fmt.Errorf("failed to create default query function: %w", err)
	}
	return func(ctx context.Context, zone dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
		switch {
		case setName.SetIdentifier != "":
			// routing policies with set identifiers are not supported by the default query function
			return h.queryDNS(ctx, zone, setName, recordType)
		case recordType == dns.TypeAWS_ALIAS_A, recordType == dns.TypeAWS_ALIAS_AAAA:
			// For AWS alias records, we query A/AAAA/TXT records by DNS queries.
			// It is expected that the DNS query will return the alias target as a TXT record.
			var queryRecordType dns.RecordType
			switch recordType {
			case dns.TypeAWS_ALIAS_A:
				queryRecordType = dns.TypeA
			case dns.TypeAWS_ALIAS_AAAA:
				queryRecordType = dns.TypeAAAA
			}
			queryResultIP := defaultQueryFunc.Query(ctx, setName, queryRecordType)
			if queryResultIP.Err != nil {
				return nil, queryResultIP.Err
			}
			queryResult := defaultQueryFunc.Query(ctx, setName, dns.TypeTXT)
			if queryResult.Err != nil {
				return nil, queryResult.Err
			}
			if queryResult.RecordSet == nil || len(queryResult.RecordSet.Records) != 1 {
				return nil, nil
			}
			// fake an alias record set from the TXT record
			queryResult.RecordSet.Records[0].Value = strings.TrimSuffix(queryResult.RecordSet.Records[0].Value, ".")
			queryResult.RecordSet.Type = recordType
			queryResult.RecordSet.TTL = 0
			return queryResult.RecordSet, nil
		default:
			// For all other record types, we can use the default query function
			queryResult := defaultQueryFunc.Query(ctx, setName, recordType)
			return queryResult.RecordSet, queryResult.Err
		}
	}, nil
}

// queryDNS queries the DNS provider for the given DNS name and record type.
func (h *handler) queryDNS(ctx context.Context, zone dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
	setName = setName.EnsureTrailingDot()
	var recordIdentifier *string
	if setName.SetIdentifier != "" {
		recordIdentifier = &setName.SetIdentifier
	}
	rrType, err := toAWSRecordType(recordType)
	if err != nil {
		return nil, err
	}
	sets, err := h.r53.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:          aws.String(zone.ZoneID().ID),
		MaxItems:              aws.Int32(1),
		StartRecordIdentifier: recordIdentifier,
		StartRecordName:       &setName.DNSName,
		StartRecordType:       rrType,
	})
	if err != nil {
		return nil, err
	}

	for _, r := range sets.ResourceRecordSets {
		if aws.ToString(r.Name) == setName.DNSName && aws.ToString(r.SetIdentifier) == setName.SetIdentifier && r.Type == rrType {
			rs := buildRecordSetFromAliasTarget(r)
			if rs == nil {
				var records []*dns.Record
				var ttl int64
				for _, rr := range r.ResourceRecords {
					records = append(records, &dns.Record{Value: aws.ToString(rr.Value)})
				}
				if r.TTL != nil {
					ttl = aws.ToInt64(r.TTL)
				}
				rs = dns.NewRecordSet(recordType, ttl, records)
			}
			rs.RoutingPolicy = h.policyContext.extractRoutingPolicy(ctx, &r)
			return rs, nil
		}
	}
	return nil, nil
}

func (h *handler) ExecuteRequests(ctx context.Context, zone provider.DNSHostedZone, reqs provider.ChangeRequests) error {
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return err
	}
	exec := newExecution(log, h, zone.ZoneID())

	var errs []error
	for _, r := range reqs.Updates {
		var err error
		if r.New == nil && r.Old == nil {
			err = fmt.Errorf("both old and new record sets are nil for %s", reqs.Name)
		} else if r.New == nil {
			err = exec.addChange(ctx, route53types.ChangeActionDelete, reqs, r.Old)
		} else if r.Old == nil {
			err = exec.addChange(ctx, route53types.ChangeActionCreate, reqs, r.New)
		} else if r.Old.Type != r.New.Type {
			err = exec.addChange(ctx, route53types.ChangeActionDelete, reqs, r.Old)
			if err != nil {
				errs = append(errs, err)
			}
			err = exec.addChange(ctx, route53types.ChangeActionCreate, reqs, r.New)
		} else {
			err = exec.addChange(ctx, route53types.ChangeActionUpsert, reqs, r.New)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	if err := exec.submitChanges(ctx, h.config.Metrics); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// AssociateVPCWithHostedZone associates a VPC with a private hosted zone
// in use by external controller
func (h *handler) AssociateVPCWithHostedZone(ctx context.Context, vpcId string, vpcRegion route53types.VPCRegion, hostedZoneId string) (*route53.AssociateVPCWithHostedZoneOutput, error) {
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
func (h *handler) DisassociateVPCFromHostedZone(ctx context.Context, vpcId string, vpcRegion route53types.VPCRegion, hostedZoneId string) (*route53.DisassociateVPCFromHostedZoneOutput, error) {
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
func (h *handler) GetZoneByName(hostedZoneId string) (*route53.GetHostedZoneOutput, error) {
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
func (h *handler) CreateVPCAssociationAuthorization(ctx context.Context, hostedZoneId string, vpcId string, vpcRegion route53types.VPCRegion) (*route53.CreateVPCAssociationAuthorizationOutput, error) {
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
func (h *handler) DeleteVPCAssociationAuthorization(ctx context.Context, hostedZoneId string, vpcId string, vpcRegion route53types.VPCRegion) (*route53.DeleteVPCAssociationAuthorizationOutput, error) {
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

func (h *handler) getLogFromContext(ctx context.Context) (logr.Logger, error) {
	log, err := logr.FromContext(ctx)
	if err != nil {
		return log, fmt.Errorf("failed to get logger from context: %w", err)
	}
	log = log.WithValues("provider", h.ProviderType())
	if h.accessKeyID != "" {
		log = log.WithValues("accessKeyID", h.accessKeyID)
	}
	return log, nil
}

func toAWSRecordType(recordType dns.RecordType) (route53types.RRType, error) {
	switch recordType {
	case dns.TypeA, dns.TypeAWS_ALIAS_A:
		return route53types.RRTypeA, nil
	case dns.TypeAAAA, dns.TypeAWS_ALIAS_AAAA:
		return route53types.RRTypeAaaa, nil
	case dns.TypeCNAME:
		return route53types.RRTypeCname, nil
	case dns.TypeTXT:
		return route53types.RRTypeTxt, nil
	case dns.TypeNS:
		return route53types.RRTypeNs, nil
	default:
		return "", fmt.Errorf("unsupported record type: %s", recordType)
	}
}

var (
	redactedRequestIDPattern = regexp.MustCompile(`RequestID: [a-z0-9-]+`)
	redactedDateTimePattern  = regexp.MustCompile(`date/time [0-9]+`)
)

// stableError converts an AWS SDK error into a stable error message without request ID
// to avoid endless status update/reconcile loop.
func stableError(err error) error {
	if err, ok := err.(*smithy.OperationError); ok {
		msg := errors.Unwrap(err.Unwrap()).Error()
		msg = redactedRequestIDPattern.ReplaceAllString(msg, "RequestID: <redacted>")
		msg = redactedDateTimePattern.ReplaceAllString(msg, "date/time: <redacted>")
		return fmt.Errorf("%s failed: %s", err.OperationName, msg)
	}
	return err
}

// ensure staticTokenRetriever implements github.com/aws/aws-sdk-go-v2/credentials/stscreds.IdentityTokenRetriever
var _ stscreds.IdentityTokenRetriever = (*staticTokenRetriever)(nil)

type staticTokenRetriever struct {
	token []byte
}

func (s *staticTokenRetriever) GetIdentityToken() ([]byte, error) {
	return s.token, nil
}

func getRoleARN(c *provider.DNSHandlerConfig) (string, error) {
	roleARN := c.GetProperty(dns.RoleARN)
	if roleARN != "" {
		return roleARN, nil
	}
	configData := c.GetProperty(securityv1alpha1constants.DataKeyConfig)
	if configData == "" {
		return "", fmt.Errorf("missing %q field in secret", securityv1alpha1constants.DataKeyConfig)
	}

	cfg := workloadidentityaws.WorkloadIdentityConfig{}
	if err := yaml.Unmarshal([]byte(configData), &cfg); err != nil {
		return "", fmt.Errorf("failed to unmarshal workload identity config: %w", err)
	}
	if err := workloadidentityaws.ValidateWorkloadIdentityConfig(&cfg, field.NewPath("config")).ToAggregate(); err != nil {
		return "", err
	}
	return cfg.RoleARN, nil
}
