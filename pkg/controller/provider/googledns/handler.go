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

package googledns

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
	config      provider.DNSHandlerConfig
	credentials *google.Credentials
	client      *http.Client
	ctx         context.Context
	service     *googledns.Service
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(logger logger.LogContext, config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	var err error

	this := &Handler{
		config: *config,
	}
	scopes := []string{
		//	"https://www.googleapis.com/auth/compute",
		//	"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/ndev.clouddns.readwrite",
		//	"https://www.googleapis.com/auth/devstorage.full_control",
	}

	json := this.config.Properties["serviceaccount.json"]
	if json == "" {
		return nil, fmt.Errorf("'serviceaccount.json' required in secret")
	}

	//c:=*http.DefaultClient
	//this.ctx=context.WithValue(config.Context,oauth2.HTTPClient,&c)
	this.ctx = config.Context

	this.credentials, err = google.CredentialsFromJSON(this.ctx, []byte(json), scopes...)
	//cfg, err:=google.JWTConfigFromJSON([]byte(json))
	if err != nil {
		return nil, fmt.Errorf("serviceaccount is invalid: %s", err)
	}
	this.client = oauth2.NewClient(this.ctx, this.credentials.TokenSource)
	//this.client=cfg.Client(ctx)

	this.service, err = googledns.New(this.client)
	if err != nil {
		return nil, err
	}

	return this, nil
}

func (this *Handler) GetZones() (provider.DNSHostedZoneInfos, error) {
	zones := provider.DNSHostedZoneInfos{}

	f := func(resp *googledns.ManagedZonesListResponse) error {
		for _, zone := range resp.ManagedZones {
			hostedZone := provider.DNSHostedZoneInfo{
				Id:     zone.Name,
				Domain: dns.NormalizeHostname(zone.DnsName),
			}
			zones = append(zones, hostedZone)
		}
		return nil
	}

	if err := this.service.ManagedZones.List(this.credentials.ProjectID).Pages(this.ctx, f); err != nil {
		return nil, err
	}

	for i, z := range zones {
		f := func(r *googledns.ResourceRecordSet) {
			if r.Type == dns.RS_NS {
				name := dns.NormalizeHostname(r.Name)
				if name != z.Domain {
					z.Forwarded = append(z.Forwarded, name)
				}
			}
		}
		this.handleRecordSets(z.Id, f)
		zones[i] = z
	}

	return zones, nil
}

func (this *Handler) handleRecordSets(zoneid string, f func(r *googledns.ResourceRecordSet)) error {
	aggr := func(resp *googledns.ResourceRecordSetsListResponse) error {
		for _, r := range resp.Rrsets {
			f(r)
		}
		return nil
	}
	return this.service.ResourceRecordSets.List(this.credentials.ProjectID, zoneid).Pages(this.ctx, aggr)
}

func (this *Handler) GetZoneState(zoneid string) (provider.DNSZoneState, error) {
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

	if err := this.handleRecordSets(zoneid, f); err != nil {
		return nil, err
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (this *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZoneInfo, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {

	exec := NewExecution(logger, this, zone)
	for _, r := range reqs {
		exec.addChange(r)
	}
	if this.config.DryRun {
		logger.Infof("no changes in dryrun mode for AWS")
		return nil
	}
	return exec.submitChanges()
}
