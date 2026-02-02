// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package google

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"
	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/google/externalaccount"
	googledns "google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/yaml"

	"github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/gcp"
	"github.com/gardener/external-dns-management/pkg/controller/provider/google/validation"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const (
	// ServiceAccountJSONField is the field in a secret where the service account JSON is stored at.
	ServiceAccountJSONField = "serviceaccount.json"
)

type Handler struct {
	provider.DefaultDNSHandler
	config      provider.DNSHandlerConfig
	cache       provider.ZoneCache
	projectID   string
	ctx         context.Context
	service     *googledns.Service
	rateLimiter flowcontrol.RateLimiter
}

const epsilon = 0.00001

var _ provider.DNSHandler = &Handler{}

var (
	// hardcoded allowed token  and service account impersonation URLs for workload identity (configurable only in next-generation dns-controller-manager)
	allowedTokenURLs                                    = []string{"https://sts.googleapis.com/v1/token"}
	allowedServiceAccountImpersonationURLRegExpsStrings = []string{`^https://iamcredentials\.googleapis\.com/v1/projects/-/serviceAccounts/.+:generateAccessToken$`}
	allowedServiceAccountImpersonationURLRegExps        []*regexp.Regexp
)

func init() {
	for _, regExp := range allowedServiceAccountImpersonationURLRegExpsStrings {
		compiled := regexp.MustCompile(regExp)
		allowedServiceAccountImpersonationURLRegExps = append(allowedServiceAccountImpersonationURLRegExps, compiled)
	}
}

func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	var err error

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *config,
		rateLimiter:       config.RateLimiter,
	}
	scopes := []string{
		googledns.NdevClouddnsReadwriteScope,
	}

	h.ctx = config.Context

	var clientOptions []option.ClientOption
	// Note: Incompatible with "WithHTTPClient"
	UAOption := option.WithUserAgent("gardener-external-dns-management")
	if config.GetProperty(securityv1alpha1constants.LabelWorkloadIdentityProvider) == "gcp" {
		// use workload identity credentials
		config.Logger.Infof("using workload identity credentials")
		externalAccountConfig, projectID, err := extractExternalAccountCredentials(config, scopes...)
		if err != nil {
			return nil, err
		}
		h.projectID = projectID
		config.Logger.Infof("using project id %s", projectID)

		ts, err := externalaccount.NewTokenSource(h.ctx, externalAccountConfig)
		if err != nil {
			return nil, err
		}
		clientOptions = []option.ClientOption{option.WithTokenSource(ts), UAOption}
	} else {
		serviceAccountJSON := h.config.Properties[ServiceAccountJSONField]
		if serviceAccountJSON == "" {
			return nil, fmt.Errorf("%q required in secret", ServiceAccountJSONField)
		}
		info, err := validation.ValidateServiceAccountJSON([]byte(serviceAccountJSON))
		if err != nil {
			return nil, err
		}
		config.Logger.Infof("using service account %s", info.ClientEmail)
		config.Logger.Infof("using project id %s", info.ProjectID)
		config.Logger.Infof("using private key id %s", info.PrivateKeyID)
		h.projectID = info.ProjectID

		jwtConfig, err := google.JWTConfigFromJSON([]byte(serviceAccountJSON), scopes...)
		if err != nil {
			return nil, fmt.Errorf("serviceaccount is invalid: %w", err)
		}
		clientOptions = []option.ClientOption{option.WithTokenSource(jwtConfig.TokenSource(h.ctx)), UAOption}
	}

	h.service, err = googledns.NewService(h.ctx, clientOptions...)
	if err != nil {
		return nil, err
	}

	h.cache, err = config.ZoneCacheFactory.CreateZoneCache(provider.CacheZoneState, config.Metrics, h.getZones, h.getZoneState)
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
	blockedZones := h.config.Options.GetBlockedZones()

	rt := provider.M_LISTZONES
	raw := []*googledns.ManagedZone{}
	f := func(resp *googledns.ManagedZonesListResponse) error {
		for _, zone := range resp.ManagedZones {
			zoneID := h.makeZoneID(zone.Name)
			if blockedZones.Contains(zoneID) {
				h.config.Logger.Infof("ignoring blocked zone id: %s", zoneID)
				continue
			}
			raw = append(raw, zone)
		}
		h.config.Metrics.AddGenericRequests(rt, 1)
		rt = provider.M_PLISTZONES
		return nil
	}

	h.config.RateLimiter.Accept()
	if err := h.service.ManagedZones.List(h.projectID).Pages(h.ctx, f); err != nil {
		return nil, err
	}

	zones := provider.DNSHostedZones{}
	for _, z := range raw {
		zoneID := h.makeZoneID(z.Name)
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zoneID, dns.NormalizeHostname(z.DnsName), "", false)
		zones = append(zones, hostedZone)
	}

	return zones, nil
}

func (h *Handler) handleRecordSets(zone provider.DNSHostedZone, f func(r *googledns.ResourceRecordSet)) error {
	rt := provider.M_LISTRECORDS
	aggr := func(resp *googledns.ResourceRecordSetsListResponse) error {
		for _, r := range resp.Rrsets {
			f(r)
		}
		h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
		rt = provider.M_PLISTRECORDS
		return nil
	}
	h.config.RateLimiter.Accept()
	projectID, zoneName := SplitZoneID(zone.Id().ID)
	return h.service.ResourceRecordSets.List(projectID, zoneName).Pages(h.ctx, aggr)
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, _ provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	f := func(r *googledns.ResourceRecordSet) {
		if dns.SupportedRecordType(r.Type) {
			if len(r.Rrdatas) > 0 {
				rs := dns.NewRecordSet(r.Type, r.Ttl, nil)
				for _, rr := range r.Rrdatas {
					rs.Add(&dns.Record{Value: rr})
				}
				dnssets.AddRecordSetFromProvider(r.Name, rs)
			} else if r.RoutingPolicy != nil && r.RoutingPolicy.Wrr != nil {
				for _, item := range r.RoutingPolicy.Wrr.Items {
					if int64(item.Weight+epsilon)*10 != int64(item.Weight*10+epsilon) {
						return // foreign as managed recordsets only use integral weights
					}
				}
				for i, item := range r.RoutingPolicy.Wrr.Items {
					if isWrrPlaceHolderItem(r.Type, item) {
						continue
					}
					rs := dns.NewRecordSet(r.Type, r.Ttl, nil)
					for _, rr := range item.Rrdatas {
						rs.Add(&dns.Record{Value: rr})
					}
					dnsSetName := dns.DNSSetName{DNSName: r.Name, SetIdentifier: fmt.Sprintf("%d", i)}
					policy := dns.NewRoutingPolicy(dns.RoutingPolicyWeighted, "weight", strconv.FormatInt(int64(item.Weight+epsilon), 10))
					dnssets.AddRecordSetFromProviderEx(dnsSetName, policy, rs)
				}
			}
		}
	}

	if err := h.handleRecordSets(zone, f); err != nil {
		return nil, err
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, _ provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h, zone)
	for _, r := range reqs {
		exec.addChange(r)
	}
	if h.config.DryRun {
		logger.Infof("no changes in dryrun mode for Google")
		return nil
	}
	return exec.submitChanges(h.config.Metrics)
}

func (h *Handler) makeZoneID(name string) string {
	return fmt.Sprintf("%s/%s", h.projectID, name)
}

func (h *Handler) getResourceRecordSet(project, managedZone, name, typ string) (*googledns.ResourceRecordSet, error) {
	h.config.RateLimiter.Accept()
	h.config.Metrics.AddGenericRequests("getrecordset", 1)
	return h.service.ResourceRecordSets.Get(project, managedZone, name, typ).Do()
}

// SplitZoneID splits the zone id into project id and zone name
func SplitZoneID(id string) (string, string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return "???", id
	}
	return parts[0], parts[1]
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

	workloadIdentityConfig, err := workloadIdentityConfigFromBytes([]byte(configData))
	if err != nil {
		return externalaccount.Config{}, "", err
	}

	externalAccountConfig, err := workloadIdentityConfig.ExtractExternalAccountCredentials(token, scopes...)
	return externalAccountConfig, workloadIdentityConfig.ProjectID, err
}

func workloadIdentityConfigFromBytes(configData []byte) (*gcp.WorkloadIdentityConfig, error) {
	cfg := &gcp.WorkloadIdentityConfig{}
	if err := yaml.Unmarshal([]byte(configData), cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workload identity config: %w", err)
	}
	if err := gcp.ValidateWorkloadIdentityConfig(cfg, field.NewPath("config"), allowedTokenURLs, allowedServiceAccountImpersonationURLRegExps).ToAggregate(); err != nil {
		return nil, err
	}
	return cfg, nil
}
