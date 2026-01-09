// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package netlify

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"github.com/netlify/open-api/go/models"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/raw"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig
	access accessor
}

var _ provider.DNSHandler = &handler{}

// NewHandler creates a new DNS handler for the Netlify DNS provider.
func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	var err error

	h := &handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *c,
	}

	apiToken, err := c.GetRequiredProperty("NETLIFY_AUTH_TOKEN", "NETLIFY_API_TOKEN")
	if err != nil {
		return nil, err
	}

	access, err := newAccess(apiToken, c.Metrics, c.RateLimiter)
	if err != nil {
		return nil, err
	}

	h.access = access

	return h, nil
}

func (h *handler) Release() {
}

func (h *handler) GetZones(ctx context.Context) ([]provider.DNSHostedZone, error) {
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return nil, err
	}

	rawZones := []models.DNSZone{}
	{
		f := func(zone models.DNSZone) (bool, error) {
			if h.isBlockedZone(zone.ID) {
				log.Info("ignoring blocked zone", "zone", zone.ID)
			} else {
				rawZones = append(rawZones, zone)
			}
			return true, nil
		}
		err := h.access.ListZones(f)
		if err != nil {
			return nil, err
		}
	}

	var zones []provider.DNSHostedZone

	for _, z := range rawZones {
		f := func(_ models.DNSRecord) (bool, error) {
			return false, nil
		}
		err := h.access.ListRecords(z.ID, f)
		if err != nil {
			if checkAccessForbidden(err) {
				// It is possible to deny access to certain zones in the account
				// As a result, z zone should not be appended to the hosted zones
				continue
			}
			return nil, err
		}
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), z.ID, z.Name, z.ID, false)
		zones = append(zones, hostedZone)
	}

	return zones, nil
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

// GetCustomQueryDNSFunc returns a custom DNS query function for the Netlify DNS provider.
func (h *handler) GetCustomQueryDNSFunc(_ dns.ZoneInfo, factory utils.QueryDNSFactoryFunc) (provider.CustomQueryDNSFunc, error) {
	defaultQueryFunc, err := factory()
	if err != nil {
		return nil, fmt.Errorf("failed to create default query function: %w", err)
	}
	return func(ctx context.Context, _ dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
		queryResult := defaultQueryFunc.Query(ctx, setName, recordType)
		return queryResult.RecordSet, queryResult.Err
	}, nil
}

func (h *handler) ExecuteRequests(ctx context.Context, zone provider.DNSHostedZone, reqs provider.ChangeRequests) error {
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return err
	}

	return raw.ExecuteRequests(ctx, log, h.access, zone, reqs, nil)
}

func checkAccessForbidden(err error) bool {
	return err != nil && strings.Contains(err.Error(), "403")
}
