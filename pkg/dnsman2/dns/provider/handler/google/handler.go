// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package google

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	"github.com/go-logr/logr"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/google/externalaccount"
	googledns "google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"

	"github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/gcp"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

const (
	// ServiceAccountJSONField is the field in a secret where the service account JSON is stored at.
	ServiceAccountJSONField = "serviceaccount.json"
)

type handler struct {
	provider.DefaultDNSHandler
	config    provider.DNSHandlerConfig
	projectID string
	service   *googledns.Service
}

type lightCredentialsFile struct {
	Type string `json:"type"`

	// Service Account fields
	ClientEmail  string `json:"client_email"`
	PrivateKeyID string `json:"private_key_id"`
	ProjectID    string `json:"project_id"`
}

const epsilon = 0.00001

var _ provider.DNSHandler = &handler{}

// NewHandler creates a new Google Cloud DNS handler based on the provided configuration.
func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	advancedOptions := c.GlobalConfig.ProviderAdvancedOptions[ProviderType]
	c.Log.Info("advanced options", "options", advancedOptions) // TODO(MartinWeindel) fix logging of advanced options

	var err error

	h := &handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *c,
	}
	scopes := []string{
		googledns.NdevClouddnsReadwriteScope,
	}

	ctx := context.Background()

	var clientOptions []option.ClientOption
	// Note: Incompatible with "WithHTTPClient"
	UAOption := option.WithUserAgent("dns-controller-manager")
	if c.GetProperty(securityv1alpha1constants.LabelWorkloadIdentityProvider) == "gcp" {
		// use workload identity credentials
		c.Log.Info("using workload identity credentials")
		externalAccountConfig, projectID, err := extractExternalAccountCredentials(c, scopes...)
		if err != nil {
			return nil, err
		}
		h.projectID = projectID
		c.Log.Info("using client for", "projectID", projectID)

		ts, err := externalaccount.NewTokenSource(ctx, externalAccountConfig)
		if err != nil {
			return nil, err
		}
		clientOptions = []option.ClientOption{option.WithTokenSource(ts), UAOption}
	} else {
		serviceAccountJSON := h.config.Properties[ServiceAccountJSONField]
		if serviceAccountJSON == "" {
			return nil, fmt.Errorf("'%s' required in secret", ServiceAccountJSONField)
		}

		info, err := validateServiceAccountJSON([]byte(serviceAccountJSON))
		if err != nil {
			return nil, err
		}
		c.Log.Info("using client for", "serviceAccount", info.ClientEmail, "projectID", info.ProjectID, "privateKeyID", info.PrivateKeyID)
		h.projectID = info.ProjectID

		jwtConfig, err := google.JWTConfigFromJSON([]byte(serviceAccountJSON), scopes...)
		if err != nil {
			return nil, fmt.Errorf("serviceaccount is invalid: %s", err)
		}
		clientOptions = []option.ClientOption{option.WithTokenSource(jwtConfig.TokenSource(ctx)), UAOption}
	}

	h.service, err = googledns.NewService(ctx, clientOptions...)
	if err != nil {
		return nil, err
	}

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

	rt := provider.MetricsRequestTypeListZones
	var raw []*googledns.ManagedZone
	f := func(resp *googledns.ManagedZonesListResponse) error {
		for _, zone := range resp.ManagedZones {
			zoneID := h.makeZoneID(zone.Name)
			if h.isBlockedZone(zoneID) {
				log.Info("ignoring blocked zone", "zone", zoneID)
				continue
			}
			raw = append(raw, zone)
		}
		h.config.Metrics.AddGenericRequests(rt, 1)
		rt = provider.MetricsRequestTypeListZonesPages
		return nil
	}

	h.config.RateLimiter.Accept()
	if err := h.service.ManagedZones.List(h.projectID).Pages(ctx, f); err != nil {
		return nil, err
	}

	var zones []provider.DNSHostedZone
	for _, z := range raw {
		zoneID := h.makeZoneID(z.Name)
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zoneID, dns.NormalizeDomainName(z.DnsName), "", false)
		zones = append(zones, hostedZone)
	}

	return zones, nil
}

func (h *handler) isBlockedZone(zoneID string) bool {
	return slices.Contains(h.getAdvancedOptions().BlockedZones, zoneID)
}

// GetCustomQueryDNSFunc returns a custom DNS query function for the Google Cloud DNS provider if the zone is private.
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
		default:
			// For all other record types, we can use the default query function
			queryResult := defaultQueryFunc.Query(ctx, setName, recordType)
			return queryResult.RecordSet, queryResult.Err
		}
	}, nil
}

// queryDNS queries the DNS provider for the given DNS name and record type.
func (h *handler) queryDNS(_ context.Context, zone dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
	projectID, zoneName, err := splitZoneID(zone.ZoneID().ID)
	if err != nil {
		return nil, err
	}
	r, err := h.getResourceRecordSet(projectID, zoneName, dns.EnsureTrailingDot(setName.DNSName), string(recordType))
	if err != nil {
		return nil, err
	}

	switch {
	case setName.SetIdentifier == "":
		// standard record set without routing policy
		return buildRecordSet(recordType, r.Ttl, r.Rrdatas), nil
	case setName.SetIdentifier != "" && r.RoutingPolicy != nil && r.RoutingPolicy.Wrr != nil:
		// weighted round-robin record set with set identifier
		return buildRecordSetWeightedRoundRobin(r, setName, recordType)
	case setName.SetIdentifier != "" && r.RoutingPolicy != nil && r.RoutingPolicy.Geo != nil:
		// geo location record set with set identifier
		return buildRecordSetGeoLocation(r, setName, recordType)
	default:
		return nil, fmt.Errorf("unsupported record set type for %s[%s] with set identifier %s", setName.DNSName, recordType, setName.SetIdentifier)
	}
}

func (h *handler) ExecuteRequests(ctx context.Context, zone provider.DNSHostedZone, reqs provider.ChangeRequests) error {
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return err
	}

	exec := newExecution(log, h, zone.ZoneID())
	var errs []error
	for _, r := range reqs.Updates {
		if r.New == nil && r.Old == nil {
			errs = append(errs, fmt.Errorf("both old and new record sets are nil for %s", reqs.Name))
		}
		if r.Old != nil {
			err := exec.addChange(deleteAction, reqs, r.Old)
			if err != nil {
				errs = append(errs, err)
			}
		}
		if r.New != nil {
			err := exec.addChange(createAction, reqs, r.New)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to execute change requests for zone %s: %w", zone.ZoneID(), errors.Join(errs...))
	}
	return exec.submitChanges(h.config.Metrics)
}

func (h *handler) makeZoneID(name string) string {
	return fmt.Sprintf("%s/%s", h.projectID, name)
}

func (h *handler) getResourceRecordSet(project, managedZone, name string, rtype string) (*googledns.ResourceRecordSet, error) {
	h.config.RateLimiter.Accept()
	h.config.Metrics.AddGenericRequests("getrecordset", 1)
	return h.service.ResourceRecordSets.Get(project, managedZone, name, rtype).Do()
}

func (h *handler) getLogFromContext(ctx context.Context) (logr.Logger, error) {
	log, err := logr.FromContext(ctx)
	if err != nil {
		return log, fmt.Errorf("failed to get logger from context: %w", err)
	}
	log = log.WithValues(
		"provider", h.ProviderType(),
		"projectID", h.projectID,
	)
	return log, nil
}

var projectIDRegexp = regexp.MustCompile(`^(?P<project>[a-z][a-z0-9-]{4,28}[a-z0-9])$`)

func validateServiceAccountJSON(data []byte) (lightCredentialsFile, error) {
	var credInfo lightCredentialsFile

	if err := json.Unmarshal(data, &credInfo); err != nil {
		return credInfo, fmt.Errorf("'serviceaccount.json' data field does not contain a valid JSON: %s", err)
	}
	if !projectIDRegexp.MatchString(credInfo.ProjectID) {
		return credInfo, fmt.Errorf("'serviceaccount.json' field 'project_id' is not a valid project")
	}
	if credInfo.Type != "service_account" {
		return credInfo, fmt.Errorf("'serviceaccount.json' field 'type' is not 'service_account'")
	}
	return credInfo, nil
}

// splitZoneID splits the zone id into project id and zone name
func splitZoneID(id string) (string, string, error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return "", id, fmt.Errorf("invalid zone ID format: %s", id)
	}
	return parts[0], parts[1], nil
}

func buildRecordSet(recordType dns.RecordType, ttl int64, rrdata []string) *dns.RecordSet {
	if len(rrdata) == 0 {
		return nil
	}

	rs := dns.NewRecordSet(recordType, ttl, nil)
	for _, rr := range rrdata {
		value := rr
		switch recordType {
		case dns.TypeCNAME:
			value = dns.NormalizeDomainName(rr)
		case dns.TypeTXT:
			if v, err := strconv.Unquote(rr); err == nil {
				value = v
			}
		}
		rs.Add(&dns.Record{Value: value})
	}
	return rs
}

func buildRecordSetWeightedRoundRobin(r *googledns.ResourceRecordSet, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
	for i, item := range r.RoutingPolicy.Wrr.Items {
		if fmt.Sprintf("%d", i) != setName.SetIdentifier {
			continue // only return the record set for the requested set identifier
		}
		if isWrrPlaceHolderItem(toRecordType(r.Type), item) {
			continue
		}
		rs := buildRecordSet(recordType, r.Ttl, item.Rrdatas)
		rs.RoutingPolicy = dns.NewRoutingPolicy(dns.RoutingPolicyWeighted, keyWeight, strconv.FormatInt(int64(item.Weight+epsilon), 10))
		return rs, nil
	}
	return nil, nil
}

func buildRecordSetGeoLocation(r *googledns.ResourceRecordSet, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
	for _, item := range r.RoutingPolicy.Geo.Items {
		if item.Location != setName.SetIdentifier {
			continue // only return the record set for the requested set identifier
		}
		rs := buildRecordSet(recordType, r.Ttl, item.Rrdatas)
		rs.RoutingPolicy = dns.NewRoutingPolicy(dns.RoutingPolicyGeoLocation, keyLocation, item.Location)
		return rs, nil
	}
	return nil, nil
}

func extractExternalAccountCredentials(config *provider.DNSHandlerConfig, scopes ...string) (externalaccount.Config, string, error) {
	configData, err := config.GetRequiredProperty(securityv1alpha1constants.DataKeyConfig)
	if err != nil {
		return externalaccount.Config{}, "", err
	}
	token, err := config.GetRequiredProperty(securityv1alpha1constants.DataKeyToken)
	if err != nil {
		return externalaccount.Config{}, "", err
	}

	workloadIdentityConfig, err := workloadIdentityConfigFromBytes(config, []byte(configData))
	if err != nil {
		return externalaccount.Config{}, "", err
	}

	externalAccountConfig, err := workloadIdentityConfig.ExtractExternalAccountCredentials(token, scopes...)
	return externalAccountConfig, workloadIdentityConfig.ProjectID, err
}

func workloadIdentityConfigFromBytes(config *provider.DNSHandlerConfig, configData []byte) (*gcp.WorkloadIdentityConfig, error) {
	if err := initialiseAllowedURLs(config.GlobalConfig.Controllers.DNSProvider.GCPWorkloadIdentityConfig); err != nil {
		return nil, err
	}
	cfg := &gcp.WorkloadIdentityConfig{}
	if err := yaml.Unmarshal([]byte(configData), cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workload identity config: %w", err)
	}
	if err := gcp.ValidateWorkloadIdentityConfig(cfg, field.NewPath("config"), allowedTokenURLs, allowedServiceAccountImpersonationURLRegExps).ToAggregate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

var (
	allowedTokenURLs                             []string
	allowedServiceAccountImpersonationURLRegExps []*regexp.Regexp
)

func initialiseAllowedURLs(cfg config.GCPWorkloadIdentityConfig) error {
	if allowedTokenURLs != nil && allowedServiceAccountImpersonationURLRegExps != nil {
		return nil
	}
	allowedTokenURLs = cfg.AllowedTokenURLs

	var regexps []*regexp.Regexp
	for _, expr := range cfg.AllowedServiceAccountImpersonationURLRegExps {
		r, err := regexp.Compile(expr)
		if err != nil {
			return fmt.Errorf("failed to compile allowed service account impersonation URL regexp '%s': %w", expr, err)
		}
		regexps = append(regexps, r)
	}
	allowedServiceAccountImpersonationURLRegExps = regexps
	return nil
}
