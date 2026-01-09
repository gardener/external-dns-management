// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package powerdns

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/go-logr/logr"
	"github.com/joeig/go-powerdns/v3"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type handler struct {
	provider.DefaultDNSHandler
	config   provider.DNSHandlerConfig
	powerdns *powerdns.Client
}

var _ provider.DNSHandler = &handler{}

// NewHandler constructs a new DNSHandler object.
func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	h := &handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *c,
	}

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

	h.powerdns = powerdns.New(server, virtualHost, powerdns.WithHeaders(headers), powerdns.WithHTTPClient(httpClient))

	return h, nil
}

func newHttpClient(insecureSkipVerify bool, trustedCaCert string) *http.Client {
	httpClient := http.DefaultClient

	if insecureSkipVerify {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 -- InsecureSkipVerify is used to allow insecure connections
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

func (h *handler) Release() {
}

func (h *handler) GetZones(ctx context.Context) ([]provider.DNSHostedZone, error) {
	var hostedZones []provider.DNSHostedZone
	zones, err := h.powerdns.Zones.List(ctx)
	if err != nil {
		return nil, err
	}

	for _, z := range zones {
		id := powerdns.StringValue(z.ID)
		if h.isBlockedZone(id) {
			continue
		}
		domain := dns.NormalizeDomainName(powerdns.StringValue(z.Name))
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), id, domain, id, false)
		hostedZones = append(hostedZones, hostedZone)
	}
	h.config.Metrics.AddGenericRequests(provider.MetricsRequestTypeListZones, 1)
	return hostedZones, nil
}

func (h *handler) getLogFromContext(ctx context.Context) (logr.Logger, error) {
	log, err := logr.FromContext(ctx)
	if err != nil {
		return log, fmt.Errorf("failed to get logger from context: %w", err)
	}
	log = log.WithValues(
		"provider", h.ProviderType(),
	)
	return log, nil
}

func (h *handler) getAdvancedOptions() config.AdvancedOptions {
	return h.config.GlobalConfig.ProviderAdvancedOptions[ProviderType]
}

func (h *handler) isBlockedZone(zoneID string) bool {
	return slices.Contains(h.getAdvancedOptions().BlockedZones, zoneID)
}

// GetCustomQueryDNSFunc returns a custom DNS query function for the PowerDNS provider.
func (h *handler) GetCustomQueryDNSFunc(_ dns.ZoneInfo, factory utils.QueryDNSFactoryFunc) (provider.CustomQueryDNSFunc, error) {
	defaultQueryFunc, err := factory()
	if err != nil {
		return nil, fmt.Errorf("failed to create default query function: %w", err)
	}
	return func(ctx context.Context, _ dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
		// For all other record types, we can use the default query function
		queryResult := defaultQueryFunc.Query(ctx, setName, recordType)
		return queryResult.RecordSet, queryResult.Err
	}, nil
}

func (h *handler) ExecuteRequests(ctx context.Context, zone provider.DNSHostedZone, reqs provider.ChangeRequests) error {
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return err
	}

	exec := newExecution(h, zone)

	var (
		succeeded, failed int
		errs              []error
	)
	for _, r := range reqs.Updates {
		if err := exec.apply(ctx, reqs.Name, r); err != nil {
			failed++
			log.Error(err, "apply failed")
			errs = append(errs, err)
		} else {
			succeeded++
		}
	}

	if succeeded > 0 {
		log.Info("Succeeded updates for records", "zone", zone.ZoneID().ID, "count", succeeded)
	}
	if failed > 0 {
		log.Info("Failed updates for records", "zone", zone.ZoneID().ID, "count", failed)
		return fmt.Errorf("%d changes failed", failed)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to execute change requests for zone %s: %w", zone.ZoneID(), errors.Join(errs...))
	}
	return nil
}
