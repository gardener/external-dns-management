// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package powerdns

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	"github.com/joeig/go-powerdns/v3"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Handler struct {
	provider.DefaultDNSHandler
	config   provider.DNSHandlerConfig
	cache    provider.ZoneCache
	ctx      context.Context
	powerdns *powerdns.Client
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *c,
	}

	h.ctx = c.Context

	server, err := c.GetRequiredProperty("Server", "server")
	if err != nil {
		return nil, err
	}

	apiKey, err := c.GetRequiredProperty("ApiKey", "apiKey")
	if err != nil {
		return nil, err
	}

	virtualHost := c.GetProperty("VirtualHost", "virtualHost")

	insecureSkipVerify, err := c.GetDefaultedBoolProperty("InsecureSkipVerify", false, "insecureSkipVerify")
	if err != nil {
		return nil, err
	}

	trustedCaCert := c.GetProperty("TrustedCaCert", "trustedCaCert")

	headers := map[string]string{"X-API-Key": apiKey}
	httpClient := newHttpClient(insecureSkipVerify, trustedCaCert)

	h.powerdns = powerdns.NewClient(server, virtualHost, headers, httpClient)

	h.cache, err = c.ZoneCacheFactory.CreateZoneCache(provider.CacheZoneState, c.Metrics, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func newHttpClient(insecureSkipVerify bool, trustedCaCert string) *http.Client {
	httpClient := http.DefaultClient

	if insecureSkipVerify {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	if trustedCaCert != "" {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM([]byte(trustedCaCert))
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:            caCertPool,
				InsecureSkipVerify: false,
				MinVersion:         tls.VersionTLS12,
			},
		}
	}

	return httpClient
}

func (h *Handler) Release() {
	h.cache.Release()
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) getZones(_ provider.ZoneCache) (provider.DNSHostedZones, error) {
	hostedZones := provider.DNSHostedZones{}
	zones, err := h.powerdns.Zones.List(h.ctx)
	if err != nil {
		return nil, err
	}

	for _, z := range zones {
		id := powerdns.StringValue(z.ID)
		domain := dns.NormalizeHostname(powerdns.StringValue(z.Name))
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), id, domain, id, false)
		hostedZones = append(hostedZones, hostedZone)
	}
	h.config.Metrics.AddGenericRequests(provider.M_LISTZONES, 1)
	return hostedZones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, _ provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	h.config.RateLimiter.Accept()

	state, err := h.powerdns.Zones.Get(h.ctx, zone.Id().ID)
	if err != nil {
		return nil, err
	}

	for _, rrset := range state.RRsets {
		h.config.Metrics.AddZoneRequests(zone.Id().ID, provider.M_LISTRECORDS, 1)
		if rrset.Type == nil {
			h.config.Logger.Warnf("Missing type for RRSet %s from Zone %s", powerdns.StringValue(rrset.Name), zone.Id().ID)
			continue
		}

		rs := dns.NewRecordSet(powerdns.StringValue((*string)(rrset.Type)), int64(powerdns.Uint32Value(rrset.TTL)), nil)
		for _, rr := range rrset.Records {
			rs.Add(&dns.Record{Value: powerdns.StringValue(rr.Content)})
		}
		dnssets.AddRecordSetFromProvider(powerdns.StringValue(rrset.Name), rs)
	}
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

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, _ provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h, zone)

	var succeeded, failed int
	for _, req := range reqs {
		rset, err := exec.buildRecordSet(req)
		if err != nil {
			if req.Done != nil {
				req.Done.SetInvalid(err)
			}
			continue
		}

		err = exec.apply(req.Action, rset, h.config.Metrics)
		if err != nil {
			failed++
			logger.Infof("Apply failed with %s", err.Error())
			if req.Done != nil {
				req.Done.Failed(err)
			}
		} else {
			succeeded++
			if req.Done != nil {
				req.Done.Succeeded()
			}
		}
	}

	if succeeded > 0 {
		logger.Infof("Succeeded updates for records in apiKey %s: %d", zone.Id(), succeeded)
	}

	if failed > 0 {
		logger.Infof("Failed updates for records in apiKey %s: %d", zone.Id(), failed)
		return fmt.Errorf("%d changes failed", failed)
	}

	return nil
}
