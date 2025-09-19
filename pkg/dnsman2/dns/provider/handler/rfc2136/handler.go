// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package rfc2136

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	miekgdns "github.com/miekg/dns"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

const (
	// maximum time DNS client can be off from server for an update to succeed
	clockSkew = 300
	tcp       = "tcp"
	// recreateOnTTLChange specifies if changing TTL of a recordset needs deletion and insert of recordset
	recreateOnTTLChange = true
)

type handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig

	nameserver    string
	zone          string
	tsigKeyname   string
	tsigSecret    string
	tsigAlgorithm string
}

var _ provider.DNSHandler = &handler{}

// NewHandler constructs a new DNSHandler object.
func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	h := &handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *c,
	}

	server, err := c.GetRequiredProperty("Server")
	if err != nil {
		return nil, err
	}
	if !strings.Contains(server, ":") {
		server += ":53"
	}
	h.nameserver = server

	zone, err := c.GetRequiredProperty("Zone")
	if err != nil {
		return nil, err
	}
	if zone != miekgdns.CanonicalName(zone) {
		return nil, fmt.Errorf("zone must be given in canonical form: '%s' instead of '%s'", miekgdns.CanonicalName(zone), h.zone)
	}
	h.zone = zone

	keyname, err := c.GetRequiredProperty("TSIGKeyName")
	if err != nil {
		return nil, err
	}
	if keyname != miekgdns.Fqdn(keyname) {
		return nil, fmt.Errorf("TSIGKeyName must end with '.'")
	}
	h.tsigKeyname = miekgdns.Fqdn(keyname)

	secret, err := c.GetRequiredProperty("TSIGSecret")
	if err != nil {
		return nil, err
	}
	h.tsigSecret = secret

	h.tsigAlgorithm, err = findTsigAlgorithm(c.GetProperty("TSIGSecretAlgorithm"))
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (h *handler) Release() {
}

func (h *handler) GetZones(_ context.Context) ([]provider.DNSHostedZone, error) {
	domainName := dns.NormalizeDomainName(h.zone)
	h.config.Metrics.AddGenericRequests(provider.MetricsRequestTypeListZones, 1)
	return []provider.DNSHostedZone{
		provider.NewDNSHostedZone(ProviderType, h.zone, domainName, h.zone, false),
	}, nil
}

// GetCustomQueryDNSFunc returns a custom DNS query function for the Alicloud DNS provider.
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

	exec := newExecution(log, h, zone)

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
