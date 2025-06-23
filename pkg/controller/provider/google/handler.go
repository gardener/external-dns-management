// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package google

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	googledns "google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Handler struct {
	provider.DefaultDNSHandler
	config      provider.DNSHandlerConfig
	cache       provider.ZoneCache
	jwtConfig   *jwt.Config
	info        lightCredentialsFile
	client      *http.Client
	ctx         context.Context
	service     *googledns.Service
	rateLimiter flowcontrol.RateLimiter
}

type lightCredentialsFile struct {
	Type string `json:"type"`

	// Service Account fields
	ClientEmail  string `json:"client_email"`
	PrivateKeyID string `json:"private_key_id"`
	ProjectID    string `json:"project_id"`
}

const epsilon = 0.00001

var _ provider.DNSHandler = &Handler{}

func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	var err error

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *config,
		rateLimiter:       config.RateLimiter,
	}
	scopes := []string{
		"https://www.googleapis.com/auth/ndev.clouddns.readwrite",
	}

	serviceAccountJSON := h.config.Properties["serviceaccount.json"]
	if serviceAccountJSON == "" {
		return nil, fmt.Errorf("'serviceaccount.json' required in secret")
	}

	h.ctx = config.Context

	h.info, err = validateServiceAccountJSON([]byte(serviceAccountJSON))
	if err != nil {
		return nil, err
	}

	config.Logger.Infof("using service account %s", h.info.ClientEmail)
	config.Logger.Infof("using project id %s", h.info.ProjectID)
	config.Logger.Infof("using private key id %s", h.info.PrivateKeyID)

	h.jwtConfig, err = google.JWTConfigFromJSON([]byte(serviceAccountJSON), scopes...)

	if err != nil {
		return nil, fmt.Errorf("serviceaccount is invalid: %s", err)
	}
	h.client = h.jwtConfig.Client(h.ctx)

	h.service, err = googledns.NewService(h.ctx, option.WithHTTPClient(h.client))
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
	if err := h.service.ManagedZones.List(h.info.ProjectID).Pages(h.ctx, f); err != nil {
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
	return fmt.Sprintf("%s/%s", h.info.ProjectID, name)
}

func (h *Handler) getResourceRecordSet(project, managedZone, name, typ string) (*googledns.ResourceRecordSet, error) {
	h.config.RateLimiter.Accept()
	h.config.Metrics.AddGenericRequests("getrecordset", 1)
	return h.service.ResourceRecordSets.Get(project, managedZone, name, typ).Do()
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

// SplitZoneID splits the zone id into project id and zone name
func SplitZoneID(id string) (string, string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return "???", id
	}
	return parts[0], parts[1]
}
