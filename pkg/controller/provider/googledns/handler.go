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
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"

	googledns "google.golang.org/api/dns/v1"
)

type Handler struct {
	config      dns.DNSHandlerConfig
	credentials *google.Credentials
	client      *http.Client
	ctx         context.Context
	service     *googledns.Service
}

var _ dns.DNSHandler = &Handler{}

func NewHandler(logger logger.LogContext, config *dns.DNSHandlerConfig) (dns.DNSHandler, error) {
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

func (this *Handler) GetZones() (dns.DNSHostedZoneInfos, error) {
	zones := dns.DNSHostedZoneInfos{}

	f := func(resp *googledns.ManagedZonesListResponse) error {
		for _, zone := range resp.ManagedZones {
			hostedZone := &dns.DNSHostedZoneInfo{
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
	return zones, nil
}

func (this *Handler) GetDNSSets(zoneid string) (dns.DNSSets, error) {
	dnssets := dns.DNSSets{}

	f := func(resp *googledns.ResourceRecordSetsListResponse) error {
		for _, r := range resp.Rrsets {
			if !dns.SupportedRecordType(r.Type) {
				continue
			}

			rs := dns.NewRecordSet(r.Type, r.Ttl, nil)
			for _, rr := range r.Rrdatas {
				rs.Add(&dns.Record{Value: rr})
			}

			dnssets.AddRecordSetFromProvider(r.Name, rs)
		}
		return nil
	}

	if err := this.service.ResourceRecordSets.List(this.credentials.ProjectID, zoneid).Pages(this.ctx, f); err != nil {
		return nil, err
	}

	return dnssets, nil
}

func (this *Handler) ExecuteRequests(logger logger.LogContext, zoneid string, reqs []*dns.ChangeRequest) error {

	exec := NewExecution(logger, this, zoneid)
	for _, r := range reqs {
		exec.addChange(r)
	}
	if this.config.DryRun {
		logger.Infof("no changes in dryrun mode for AWS")
		return nil
	}
	return exec.submitChanges()
}
