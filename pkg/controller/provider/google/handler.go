// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package google

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googledns "google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Handler struct {
	provider.DefaultDNSHandler
	config      provider.DNSHandlerConfig
	cache       provider.ZoneCache
	credentials *google.Credentials
	client      *http.Client
	ctx         context.Context
	service     *googledns.Service
	rateLimiter flowcontrol.RateLimiter
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
		//	"https://www.googleapis.com/auth/compute",
		//	"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/ndev.clouddns.readwrite",
		//	"https://www.googleapis.com/auth/devstorage.full_control",
	}

	json := h.config.Properties["serviceaccount.json"]
	if json == "" {
		return nil, fmt.Errorf("'serviceaccount.json' required in secret")
	}

	// c:=*http.DefaultClient
	// h.ctx=context.WithValue(config.Context,oauth2.HTTPClient,&c)
	h.ctx = config.Context

	h.credentials, err = google.CredentialsFromJSON(h.ctx, []byte(json), scopes...)
	// cfg, err:=google.JWTConfigFromJSON([]byte(json))
	if err != nil {
		return nil, fmt.Errorf("serviceaccount is invalid: %s", err)
	}
	h.client = oauth2.NewClient(h.ctx, h.credentials.TokenSource)
	// h.client=cfg.Client(ctx)

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
	if err := h.service.ManagedZones.List(h.credentials.ProjectID).Pages(h.ctx, f); err != nil {
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
	return fmt.Sprintf("%s/%s", h.credentials.ProjectID, name)
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
