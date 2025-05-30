// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package infoblox

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/gardener/controller-manager-library/pkg/logger"
	ibclient "github.com/infobloxopen/infoblox-go-client/v2"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
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
	Host            *string            `json:"host,omitempty"`
	Port            *int               `json:"port,omitempty"`
	SSLVerify       *bool              `json:"sslVerify,omitempty"`
	Version         *string            `json:"version,omitempty"`
	View            *string            `json:"view,omitempty"`
	PoolConnections *int               `json:"httpPoolConnections,omitempty"`
	RequestTimeout  *int               `json:"httpRequestTimeout,omitempty"`
	CaCert          *string            `json:"caCert,omitempty"`
	MaxResults      int                `json:"maxResults,omitempty"`
	ProxyURL        *string            `json:"proxyUrl,omitempty"`
	ExtAttrs        *map[string]string `json:"extAttrs,omitempty"`
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
		Host:    *infobloxConfig.Host,
		Port:    strconv.Itoa(*infobloxConfig.Port),
		Version: *infobloxConfig.Version,
	}

	authCfg := ibclient.AuthConfig{
		Username: username,
		Password: password,
	}

	verify := "true"
	if infobloxConfig.SSLVerify != nil {
		verify = strconv.FormatBool(*infobloxConfig.SSLVerify)
	}

	transportConfig := ibclient.NewTransportConfig(
		verify,
		*infobloxConfig.RequestTimeout,
		*infobloxConfig.PoolConnections,
	)
	if infobloxConfig.CaCert != nil && verify == "true" {
		transportConfig.CertPool, err = h.createCertPool([]byte(*infobloxConfig.CaCert))
		if err != nil {
			return nil, fmt.Errorf("cannot create cert pool for cacert: %w", err)
		}
	}
	if infobloxConfig.ProxyURL != nil && *infobloxConfig.ProxyURL != "" {
		u, err := url.Parse(*infobloxConfig.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("parsing proxy url failed: %w", err)
		}
		transportConfig.ProxyUrl = u
	}

	rb, err := ibclient.NewWapiRequestBuilder(hostConfig, authCfg)
	if err != nil {
		return nil, err
	}
	var requestBuilder ibclient.HttpRequestBuilder = rb
	if infobloxConfig.MaxResults != 0 {
		// wrap request builder which sets _max_results parameter on GET requests
		requestBuilder = NewMaxResultsRequestBuilder(infobloxConfig.MaxResults, requestBuilder)
	}
	client, err := ibclient.NewConnector(hostConfig, authCfg, transportConfig, requestBuilder, &ibclient.WapiHttpRequestor{})
	if err != nil {
		return nil, err
	}

	ea := make(ibclient.EA)

	if infobloxConfig.ExtAttrs != nil {
		for k, v := range *infobloxConfig.ExtAttrs {
			ea[k] = v
		}
	}

	h.access = NewAccess(client, requestBuilder, *h.infobloxConfig.View, config.Metrics, ea)

	h.ZoneCache, err = config.ZoneCacheFactory.CreateZoneCache(provider.CacheZonesOnly, config.Metrics, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return h, nil
}

// Infoblox does not support zone forwarding???
// Just removed the forwarding stuff from code

func (h *Handler) getZones(_ provider.ZoneCache) (provider.DNSHostedZones, error) {
	var raw []ibclient.ZoneAuth
	h.config.Metrics.AddGenericRequests(provider.M_LISTZONES, 1)
	obj := ibclient.NewZoneAuth(ibclient.ZoneAuth{})
	err := h.access.GetObject(obj, "", &ibclient.QueryParams{}, &raw)
	if filterNotFound(err) != nil {
		return nil, err
	}

	blockedZones := h.config.Options.GetBlockedZones()
	zones := provider.DNSHostedZones{}
	for _, z := range raw {
		if blockedZones.Contains(z.Ref) {
			h.config.Logger.Infof("ignoring blocked zone id: %s", z.Ref)
			continue
		}

		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), z.Ref, dns.NormalizeHostname(z.Fqdn), z.Fqdn, false)
		zones = append(zones, hostedZone)
	}
	return zones, nil
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, _ provider.ZoneCache) (provider.DNSZoneState, error) {
	state := raw.NewState()
	rt := provider.M_LISTRECORDS

	params := ibclient.NewQueryParams(false, map[string]string{"view": *h.infobloxConfig.View, "zone": zone.Key()})

	h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
	var resA []RecordA
	err := h.access.GetObject(ibclient.NewEmptyRecordA(), "", params, &resA)
	if filterNotFound(err) != nil {
		return nil, fmt.Errorf("could not fetch A records from zone '%s': %s", zone.Key(), err)
	}
	for _, res := range resA {
		state.AddRecord((&res).Copy())
	}

	h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
	var resAAAA []RecordAAAA
	err = h.access.GetObject(ibclient.NewEmptyRecordAAAA(), "", params, &resAAAA)
	if filterNotFound(err) != nil {
		return nil, fmt.Errorf("could not fetch AAAA records from zone '%s': %s", zone.Key(), err)
	}
	for _, res := range resAAAA {
		state.AddRecord((&res).Copy())
	}

	h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
	var resC []RecordCNAME
	err = h.access.GetObject(ibclient.NewEmptyRecordCNAME(), "", params, &resC)
	if filterNotFound(err) != nil {
		return nil, fmt.Errorf("could not fetch CNAME records from zone '%s': %s", zone.Key(), err)
	}
	for _, res := range resC {
		state.AddRecord((&res).Copy())
	}

	h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
	var resT []RecordTXT
	err = h.access.GetObject(ibclient.NewEmptyRecordTXT(), "", params, &resT)
	if filterNotFound(err) != nil {
		return nil, fmt.Errorf("could not fetch TXT records from zone '%s': %s", zone.Key(), err)
	}
	for _, res := range resT {
		state.AddRecord((&res).Copy())
	}

	state.CalculateDNSSets()
	return state, nil
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := raw.ExecuteRequests(h.ctx, logger, &h.config, h.access, zone, state, reqs, nil)
	h.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) GetRecordSet(zone provider.DNSHostedZone, dnsName, recordType string) (provider.DedicatedRecordSet, error) {
	rs, err := h.access.GetRecordSet(h.ctx, dnsName, recordType, zone)
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
		err = h.access.CreateRecord(h.ctx, r0, zone)
		if err != nil {
			return err
		}
	}
	return err
}

func (h *Handler) DeleteRecordSet(_ logger.LogContext, zone provider.DNSHostedZone, rs provider.DedicatedRecordSet) error {
	for _, r := range rs {
		if r.(Record).GetId() != "" {
			err := h.access.DeleteRecord(h.ctx, r.(Record), zone)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) createCertPool(cert []byte) (*x509.CertPool, error) {
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(cert) {
		return nil, fmt.Errorf("cannot append certificate")
	}
	return caPool, nil
}

func filterNotFound(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*ibclient.NotFoundError); ok {
		return nil
	}
	return err
}
