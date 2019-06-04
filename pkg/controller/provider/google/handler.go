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
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"net/http"

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
	metrics     provider.Metrics
	service     *googledns.Service
}

type googleProviderData struct {
	forwardedDomains map[string][]string
}

func NewProviderData() *googleProviderData {
	return &googleProviderData{forwardedDomains: map[string][]string{}}
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(logger logger.LogContext, config *provider.DNSHandlerConfig, metrics provider.Metrics) (provider.DNSHandler, error) {
	var err error

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *config,
		metrics:           metrics,
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

	h.cache, err = provider.NewZoneCache(config.CacheConfig, metrics, NewProviderData())
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones(h.getZones)
}

func (h *Handler) getZones(data interface{}) (provider.DNSHostedZones, error) {
	rt := provider.M_LISTZONES
	raw := []*googledns.ManagedZone{}
	f := func(resp *googledns.ManagedZonesListResponse) error {
		for _, zone := range resp.ManagedZones {
			raw = append(raw, zone)
		}
		h.metrics.AddRequests(rt, 1)
		rt = provider.M_PLISTZONES
		return nil
	}

	if err := h.service.ManagedZones.List(h.credentials.ProjectID).Pages(h.ctx, f); err != nil {
		return nil, err
	}

	zones := provider.DNSHostedZones{}
	for _, z := range raw {
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(),
			z.Name, dns.NormalizeHostname(z.DnsName), "", []string{})

		// call GetZoneState for side effect to calculate forwarded domains
		_, err := h.GetZoneState(hostedZone)
		if err == nil {
			providerData := data.(*googleProviderData)
			hostedZone = provider.CopyDNSHostedZone(hostedZone, providerData.forwardedDomains[hostedZone.Id()])
		}

		zones = append(zones, hostedZone)
	}

	return zones, nil
}

func (h *Handler) handleRecordSets(zoneid string, domain string, providerData *googleProviderData, f func(r *googledns.ResourceRecordSet)) error {
	rt := provider.M_LISTRECORDS
	forwarded := []string{}
	aggr := func(resp *googledns.ResourceRecordSetsListResponse) error {
		for _, r := range resp.Rrsets {
			f(r)
			if r.Type == dns.RS_NS {
				if r.Name != domain {
					forwarded = append(forwarded, dns.NormalizeHostname(r.Name))
				}
			}
		}
		h.metrics.AddRequests(rt, 1)
		rt = provider.M_PLISTRECORDS
		return nil
	}
	return h.service.ResourceRecordSets.List(h.credentials.ProjectID, zoneid).Pages(h.ctx, aggr)
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone, h.getZoneState)
}

func (h *Handler) getZoneState(data interface{}, zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
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

	domain := zone.Id() + "."
	if err := h.handleRecordSets(zone.Id(), domain, data.(*googleProviderData), f); err != nil {
		return nil, err
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	if err == nil {
		h.cache.ExecuteRequests(zone, reqs)
	} else {
		h.cache.DeleteZoneState(zone)
	}
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
	return exec.submitChanges(h.metrics)
}
