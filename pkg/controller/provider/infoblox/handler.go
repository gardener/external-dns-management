/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package infoblox

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"

	ibclient "github.com/infobloxopen/infoblox-go-client/v2"
)

type Handler struct {
	provider.ZoneCache
	provider.DefaultDNSHandler
	config         provider.DNSHandlerConfig
	infobloxConfig *InfobloxConfig
	access         *access
	ctx            context.Context
}

type InfobloxConfig struct {
	Host            *string `json:"host,omitempty"`
	Port            *int    `json:"port,omitempty"`
	SSLVerify       *bool   `json:"sslVerify,omitempty"`
	Version         *string `json:"version,omitempty"`
	View            *string `json:"view,omitempty"`
	PoolConnections *int    `json:"httpPoolConnections,omitempty"`
	RequestTimeout  *int    `json:"httpRequestTimeout,omitempty"`
	CaCert          *string `json:"caCert,omitempty"`
	MaxResults      int     `json:"maxResults,omitempty"`
	ProxyURL        *string `json:"proxyUrl,omitempty"`
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {

	infobloxConfig := &InfobloxConfig{}
	if config.Config != nil {
		err := json.Unmarshal(config.Config.Raw, infobloxConfig)
		if err != nil {
			return nil, fmt.Errorf("unmarshal infoblox providerConfig failed with: %s", err)
		}
	}

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *config,
		infobloxConfig:    infobloxConfig,
		ctx:               config.Context,
	}

	username, err := config.GetRequiredProperty("USERNAME", "username")
	if err != nil {
		return nil, err
	}
	password, err := config.GetRequiredProperty("PASSWORD", "password")
	if err != nil {
		return nil, err
	}

	if err := config.FillDefaultedProperty(&infobloxConfig.Version, "2.10", "VERSION", "version"); err != nil {
		return nil, err
	}
	if err := config.FillDefaultedProperty(&infobloxConfig.View, "default", "VIEW", "view"); err != nil {
		return nil, err
	}
	if err := config.FillRequiredProperty(&infobloxConfig.Host, "HOST", "host"); err != nil {
		return nil, err
	}
	if err := config.FillDefaultedIntProperty(&infobloxConfig.Port, 443, "PORT", "port"); err != nil {
		return nil, err
	}
	if err := config.FillDefaultedIntProperty(&infobloxConfig.PoolConnections, 10, "HTTP_POOL_CONNECTIONS", "http_pool_connections", "httpPoolConnections"); err != nil {
		return nil, err
	}
	if err := config.FillDefaultedIntProperty(&infobloxConfig.RequestTimeout, 60, "HTTP_REQUEST_TIMEOUT", "http_request_timeout", "httpRequestTimeout"); err != nil {
		return nil, err
	}
	if err := config.FillDefaultedProperty(&infobloxConfig.ProxyURL, "", "PROXY_URL", "proxy_url", "proxyUrl"); err != nil {
		return nil, err
	}
	if err := config.FillDefaultedProperty(&infobloxConfig.CaCert, "", "CA_CERT", "ca_cert", "caCert"); err != nil {
		return nil, err
	}
	if err := config.FillDefaultedBoolProperty(&infobloxConfig.SSLVerify, true, "SSL_VERIFY", "ssl_verify", "sslVerify"); err != nil {
		return nil, err
	}

	config.Logger.Infof("creating infoblox handler for %s", *infobloxConfig.Host)

	hostConfig := ibclient.HostConfig{
		Host:     *infobloxConfig.Host,
		Port:     strconv.Itoa(*infobloxConfig.Port),
		Version:  *infobloxConfig.Version,
		Username: username,
		Password: password,
	}

	verify := "true"
	if infobloxConfig.SSLVerify != nil {
		verify = strconv.FormatBool(*infobloxConfig.SSLVerify)
	}

	if infobloxConfig.CaCert != nil && verify == "true" {
		tmpfile, err := ioutil.TempFile("", "cacert")
		if err != nil {
			return nil, fmt.Errorf("cannot create temporary file for cacert: %w", err)
		}
		defer os.Remove(tmpfile.Name())
		if _, err := tmpfile.Write([]byte(*infobloxConfig.CaCert)); err != nil {
			return nil, fmt.Errorf("cannot write temporary file for cacert: %w", err)
		}
		if err := tmpfile.Close(); err != nil {
			return nil, fmt.Errorf("cannot close temporary file for cacert: %w", err)
		}
		verify = tmpfile.Name()
	}
	transportConfig := ibclient.NewTransportConfig(
		verify,
		*infobloxConfig.RequestTimeout,
		*infobloxConfig.PoolConnections,
	)
	if infobloxConfig.ProxyURL != nil && *infobloxConfig.ProxyURL != "" {
		u, err := url.Parse(*infobloxConfig.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("parsing proxy url failed: %w", err)
		}
		transportConfig.ProxyUrl = u
	}

	var requestBuilder ibclient.HttpRequestBuilder = &ibclient.WapiRequestBuilder{}
	if infobloxConfig.MaxResults != 0 {
		// wrap request builder which sets _max_results parameter on GET requests
		requestBuilder = NewMaxResultsRequestBuilder(infobloxConfig.MaxResults, requestBuilder)
	}
	client, err := ibclient.NewConnector(hostConfig, transportConfig, requestBuilder, &ibclient.WapiHttpRequestor{})
	if err != nil {
		return nil, err
	}

	h.access = NewAccess(client, *h.infobloxConfig.View, config.Metrics)

	h.ZoneCache, err = config.ZoneCacheFactory.CreateZoneCache(provider.CacheZonesOnly, config.Metrics, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return h, nil
}

// Infoblox does not support zone forwarding???
// Just removed the forwarding stuff from code

func (h *Handler) getZones(cache provider.ZoneCache) (provider.DNSHostedZones, error) {
	var raw []ibclient.ZoneAuth
	h.config.Metrics.AddGenericRequests(provider.M_LISTZONES, 1)
	obj := ibclient.NewZoneAuth(ibclient.ZoneAuth{})
	err := h.access.GetObject(obj, "", &ibclient.QueryParams{}, &raw)
	if err != nil {
		return nil, err
	}

	blockedZones := h.config.Options.AdvancedOptions.GetBlockedZones()
	zones := provider.DNSHostedZones{}
	for _, z := range raw {
		if blockedZones.Contains(z.Ref) {
			h.config.Logger.Infof("ignoring blocked zone id: %s", z.Ref)
			continue
		}

		h.config.Metrics.AddZoneRequests(z.Ref, provider.M_LISTRECORDS, 1)
		var resN []RecordNS
		objN := ibclient.NewRecordNS(
			ibclient.RecordNS{
				Zone: z.Fqdn,
				View: *h.infobloxConfig.View,
			},
		)
		err = h.access.GetObject(objN, "", &ibclient.QueryParams{}, &resN)
		if err != nil {
			return nil, fmt.Errorf("could not fetch NS records from zone '%s': %s", z.Fqdn, err)
		}
		forwarded := []string{}
		for _, res := range resN {
			if res.Name != z.Fqdn {
				forwarded = append(forwarded, res.Name)
			}
		}
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), z.Ref, dns.NormalizeHostname(z.Fqdn), z.Fqdn, forwarded, false)
		zones = append(zones, hostedZone)
	}
	return zones, nil
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	state := raw.NewState()
	rt := provider.M_LISTRECORDS

	h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
	var resA []RecordA
	objA := ibclient.NewEmptyRecordA()
	objA.Zone = zone.Key()
	objA.View = *h.infobloxConfig.View
	err := h.access.GetObject(objA, "", &ibclient.QueryParams{}, &resA)
	if err != nil {
		return nil, fmt.Errorf("could not fetch A records from zone '%s': %s", zone.Key(), err)
	}
	for _, res := range resA {
		state.AddRecord((&res).Copy())
	}

	h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
	var resAAAA []RecordAAAA
	objAAAA := ibclient.NewEmptyRecordAAAA()
	objAAAA.Zone = zone.Key()
	objAAAA.View = *h.infobloxConfig.View
	err = h.access.GetObject(objAAAA, "", &ibclient.QueryParams{}, &resAAAA)
	if err != nil {
		return nil, fmt.Errorf("could not fetch AAAA records from zone '%s': %s", zone.Key(), err)
	}
	for _, res := range resAAAA {
		state.AddRecord((&res).Copy())
	}

	h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
	var resC []RecordCNAME
	objC := ibclient.NewEmptyRecordCNAME()
	objC.Zone = zone.Key()
	objC.View = *h.infobloxConfig.View
	err = h.access.GetObject(objC, "", &ibclient.QueryParams{}, &resC)
	if err != nil {
		return nil, fmt.Errorf("could not fetch CNAME records from zone '%s': %s", zone.Key(), err)
	}
	for _, res := range resC {
		state.AddRecord((&res).Copy())
	}

	h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
	var resT []RecordTXT
	objT := ibclient.NewRecordTXT(
		ibclient.RecordTXT{
			Zone: zone.Key(),
			View: *h.infobloxConfig.View,
		},
	)
	err = h.access.GetObject(objT, "", &ibclient.QueryParams{}, &resT)
	if err != nil {
		return nil, fmt.Errorf("could not fetch TXT records from zone '%s': %s", zone.Key(), err)
	}
	for _, res := range resT {
		state.AddRecord((&res).Copy())
	}

	state.CalculateDNSSets()
	return state, nil
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := raw.ExecuteRequests(logger, &h.config, h.access, zone, state, reqs)
	h.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) GetRecordSet(zone provider.DNSHostedZone, dnsName, recordType string) (provider.DedicatedRecordSet, error) {
	rs, err := h.access.GetRecordSet(dnsName, recordType, zone)
	if err != nil {
		return nil, err
	}
	d := provider.DedicatedRecordSet{}
	for _, r := range rs {
		d = append(d, r)
	}
	return d, nil
}

func (h *Handler) CreateOrUpdateRecordSet(logger logger.LogContext, zone provider.DNSHostedZone, old, new provider.DedicatedRecordSet) error {
	err := h.DeleteRecordSet(logger, zone, old)
	if err != nil {
		return err
	}
	for _, r := range new {
		r0 := h.access.NewRecord(r.GetDNSName(), r.GetType(), r.GetValue(), zone, int64(r.GetTTL()))
		err = h.access.CreateRecord(r0, zone)
		if err != nil {
			return err
		}
	}
	return err
}

func (h *Handler) DeleteRecordSet(logger logger.LogContext, zone provider.DNSHostedZone, rs provider.DedicatedRecordSet) error {
	for _, r := range rs {
		if r.(Record).GetId() != "" {
			err := h.access.DeleteRecord(r.(Record), zone)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
