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

package google

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dns/provider"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/gardener/controller-manager-library/pkg/logger"

	"github.com/gardener/external-dns-management/pkg/dns"

	googledns "google.golang.org/api/dns/v1"
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

	//c:=*http.DefaultClient
	//h.ctx=context.WithValue(config.Context,oauth2.HTTPClient,&c)
	h.ctx = config.Context

	h.credentials, err = google.CredentialsFromJSON(h.ctx, []byte(json), scopes...)
	//cfg, err:=google.JWTConfigFromJSON([]byte(json))
	if err != nil {
		return nil, fmt.Errorf("serviceaccount is invalid: %s", err)
	}
	h.client = oauth2.NewClient(h.ctx, h.credentials.TokenSource)
	//h.client=cfg.Client(ctx)

	h.service, err = googledns.New(h.client)
	if err != nil {
		return nil, err
	}

	forwardedDomains := provider.NewForwardedDomainsHandlerData()
	h.cache, err = provider.NewZoneCache(config.CacheConfig, config.Metrics, forwardedDomains, h.getZones, h.getZoneState)
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
	raw := []*googledns.ManagedZone{}
	f := func(resp *googledns.ManagedZonesListResponse) error {
		for _, zone := range resp.ManagedZones {
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
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zoneID, dns.NormalizeHostname(z.DnsName), "", []string{}, false)

		// call GetZoneState for side effect to calculate forwarded domains
		_, err := cache.GetZoneState(hostedZone, false)
		if err == nil {
			forwarded := cache.GetHandlerData().(*provider.ForwardedDomainsHandlerData).GetForwardedDomains(hostedZone.Id())
			if forwarded != nil {
				hostedZone = provider.CopyDNSHostedZone(hostedZone, forwarded)
			}
		}

		zones = append(zones, hostedZone)
	}

	return zones, nil
}

func (h *Handler) handleRecordSets(zone provider.DNSHostedZone, f func(r *googledns.ResourceRecordSet)) ([]string, error) {
	rt := provider.M_LISTRECORDS
	forwarded := []string{}
	aggr := func(resp *googledns.ResourceRecordSetsListResponse) error {
		for _, r := range resp.Rrsets {
			f(r)
			if r.Type == dns.RS_NS {
				name := dns.NormalizeHostname(r.Name)
				if name != zone.Domain() {
					forwarded = append(forwarded, name)
				}
			}
		}
		h.config.Metrics.AddZoneRequests(zone.Id(), rt, 1)
		rt = provider.M_PLISTRECORDS
		return nil
	}
	h.config.RateLimiter.Accept()
	projectID, zoneName := SplitZoneID(zone.Id())
	err := h.service.ResourceRecordSets.List(projectID, zoneName).Pages(h.ctx, aggr)
	return forwarded, err
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone, forceUpdate bool) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone, forceUpdate)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	f := func(r *googledns.ResourceRecordSet) {
		if dns.SupportedRecordType(r.Type) {
			rs := dns.NewRecordSet(r.Type, r.Ttl, nil)
			for _, rr := range r.Rrdatas {
				rs.Add(&dns.Record{Value: rr})
			}
			dnssets.AddRecordSetFromProvider(r.Name, rs)
		}
	}

	forwarded, err := h.handleRecordSets(zone, f)
	if err != nil {
		return nil, err
	}
	cache.GetHandlerData().(*provider.ForwardedDomainsHandlerData).SetForwardedDomains(zone.Id(), forwarded)

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ReportZoneStateConflict(zone provider.DNSHostedZone, err error) bool {
	return h.cache.ReportZoneStateConflict(zone, err)
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h, zone)
	for _, r := range reqs {
		exec.addChange(r)
	}
	if h.config.DryRun {
		logger.Infof("no changes in dryrun mode for AWS")
		return nil
	}
	return exec.submitChanges(h.config.Metrics)
}

func (h *Handler) makeZoneID(name string) string {
	return fmt.Sprintf("%s/%s", h.credentials.ProjectID, name)
}

// SplitZoneID splits the zone id into project id and zone name
func SplitZoneID(id string) (string, string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return "???", id
	}
	return parts[0], parts[1]
}
