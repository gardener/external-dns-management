/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. h file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package openstack

import (
	"context"
	"fmt"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gophercloud/gophercloud/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/openstack/dns/v2/zones"
	"github.com/gophercloud/utils/openstack/clientconfig"
)

// Handler is the main DNSHandler struct.
type Handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig
	cache  provider.ZoneCache
	ctx    context.Context

	client designateClientInterface
}

var _ provider.DNSHandler = &Handler{}

// NewHandler constructs a new DNSHandler object.
func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	authConfig, err := readAuthConfig(config)
	if err != nil {
		return nil, err
	}

	serviceClient, err := createDesignateServiceClient(config.Logger, authConfig)
	if err != nil {
		return nil, err
	}

	h := Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *config,
		ctx:               config.Context,
		client:            designateClient{serviceClient: serviceClient, metrics: config.Metrics},
	}

	h.cache, err = provider.NewZoneCache(config.CacheConfig, config.Metrics, nil, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return &h, nil
}

func readAuthConfig(c *provider.DNSHandlerConfig) (*clientAuthConfig, error) {
	authURL, err := c.GetRequiredProperty("OS_AUTH_URL")
	if err != nil {
		return nil, err
	}
	username, err := c.GetRequiredProperty("OS_USERNAME", "username")
	if err != nil {
		return nil, err
	}
	password, err := c.GetRequiredProperty("OS_PASSWORD", "password")
	if err != nil {
		return nil, err
	}

	domainName := c.GetProperty("OS_DOMAIN_NAME", "domainName")
	domainID := c.GetProperty("OS_DOMAIN_ID", "domainID")
	projectName := c.GetProperty("OS_PROJECT_NAME", "tenantName")
	projectID := c.GetProperty("OS_PROJECT_ID", "tenantID")
	userDomainName := c.GetProperty("OS_USER_DOMAIN_NAME", "userDomainName")
	userDomainID := c.GetProperty("OS_USER_DOMAIN_ID", "userDomainID")
	// optional restriction to region
	regionName := c.GetProperty("OS_REGION_NAME")
	// optional CA Certificate for keystone
	caCert := c.GetProperty("CACERT", "caCert")
	// optional Client Certificate
	clientCert := c.GetProperty("CLIENTCERT", "clientCert")
	clientKey := c.GetProperty("CLIENTKEY", "clientKey")
	insecure := strings.ToLower(c.GetProperty("INSECURE", "insecure"))

	authConfig := clientAuthConfig{
		AuthInfo: clientconfig.AuthInfo{
			AuthURL:        authURL,
			Username:       username,
			Password:       password,
			DomainName:     domainName,
			DomainID:       domainID,
			ProjectName:    projectName,
			ProjectID:      projectID,
			UserDomainID:   userDomainID,
			UserDomainName: userDomainName,
		},
		RegionName: regionName,
		CACert:     caCert,
		ClientCert: clientCert,
		ClientKey:  clientKey,
		Insecure:   insecure=="true" || insecure=="yes",
	}

	return &authConfig, nil
}

// Release releases the zone cache.
func (h *Handler) Release() {
	h.cache.Release()
}

// GetZones returns a list of hosted zones from the cache.
func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) getZones(cache provider.ZoneCache) (provider.DNSHostedZones, error) {
	hostedZones := provider.DNSHostedZones{}

	zoneHandler := func(zone *zones.Zone) error {
		forwarded := h.collectForwardedSubzones(zone)

		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zone.ID, dns.NormalizeHostname(zone.Name), "", forwarded, false)
		hostedZones = append(hostedZones, hostedZone)
		return nil
	}

	if err := h.client.ForEachZone(zoneHandler); err != nil {
		return nil, fmt.Errorf("listing DNS zones failed. Details: %s", err.Error())
	}

	return hostedZones, nil
}

func (h *Handler) collectForwardedSubzones(zone *zones.Zone) []string {
	forwarded := []string{}

	recordSetHandler := func(recordSet *recordsets.RecordSet) error {
		if recordSet.Type == "NS" && recordSet.Name != zone.Name && len(recordSet.Records) > 0 {
			fullDomainName := dns.NormalizeHostname(recordSet.Name)
			forwarded = append(forwarded, fullDomainName)
		}
		return nil
	}

	if err := h.client.ForEachRecordSetFilterByTypeAndName(zone.ID, "NS", "", recordSetHandler); err != nil {
		logger.Infof("Failed fetching NS records for %s: %s", zone.Name, err.Error())
		// just ignoring it
		return forwarded
	}

	return forwarded
}

// GetZoneState returns the state for a given zone.
func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	recordSetHandler := func(recordSet *recordsets.RecordSet) error {
		switch recordSet.Type {
		case dns.RS_A, dns.RS_CNAME, dns.RS_TXT:
			rs := dns.NewRecordSet(recordSet.Type, int64(recordSet.TTL), nil)
			for _, record := range recordSet.Records {
				value := record
				if recordSet.Type == dns.RS_CNAME {
					value = dns.NormalizeHostname(value)
				}
				rs.Add(&dns.Record{Value: value})
			}
			dnssets.AddRecordSetFromProvider(recordSet.Name, rs)
		}
		return nil
	}

	if err := h.client.ForEachRecordSet(zone.Id(), recordSetHandler); err != nil {
		return nil, fmt.Errorf("Listing DNS zones failed for %s. Details: %s", zone.Id(), err.Error())
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ReportZoneStateConflict(zone provider.DNSHostedZone, err error) bool {
	return h.cache.ReportZoneStateConflict(zone, err)
}

// ExecuteRequests applies a given change request to a given hosted zone.
func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h, zone)

	var succeeded, failed int
	for _, r := range reqs {
		status, rset := exec.buildRecordSet(r)
		if status == bsEmpty || status == bsDryRun {
			continue
		}

		err := exec.apply(r.Action, rset)
		if err != nil {
			failed++
			logger.Infof("Apply failed with %s", err.Error())
			if r.Done != nil {
				r.Done.Failed(err)
			}
		} else {
			succeeded++
			if r.Done != nil {
				r.Done.Succeeded()
			}
		}
	}

	if h.config.DryRun {
		logger.Infof("no changes in dryrun mode for OpenStack")
		return nil
	}

	if succeeded > 0 {
		logger.Infof("Succeeded updates for records in zone %s: %d", zone.Domain(), succeeded)
	}
	if failed > 0 {
		logger.Infof("Failed updates for records in zone %s: %d", zone.Domain(), failed)
		return fmt.Errorf("%d changes failed", failed)
	}

	return nil
}
