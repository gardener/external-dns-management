// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cloudflare

import (
	"context"
	"fmt"
	"slices"

	cloudflarezones "github.com/cloudflare/cloudflare-go/v6/zones"
	"github.com/go-logr/logr"

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

// NewHandler creates a new DNSHandler for the Cloudflare DNS provider.
func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	var err error

	h := &handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *c,
	}

	apiToken, err := c.GetRequiredProperty("CLOUDFLARE_API_TOKEN", "apiToken")
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

	rawZones := []cloudflarezones.Zone{}
	{
		f := func(zone cloudflarezones.Zone) (bool, error) {
			if h.isBlockedZone(zone.ID) {
				log.Info("ignoring blocked zone", "zone", zone.ID)
			} else {
				rawZones = append(rawZones, zone)
			}
			return true, nil
		}
		err := h.access.ListZones(ctx, f)
		if err != nil {
			return nil, err
		}
	}

	var zones []provider.DNSHostedZone

	for _, z := range rawZones {
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

// GetCustomQueryDNSFunc returns a custom DNS query function for the Cloudflare DNS provider.
func (h *handler) GetCustomQueryDNSFunc(zone dns.ZoneInfo, factory utils.QueryDNSFactoryFunc) (provider.CustomQueryDNSFunc, error) {
	defaultQueryFunc, err := factory()
	if err != nil {
		return nil, fmt.Errorf("failed to create default query function: %w", err)
	}
	// routing policies with set identifiers are not supported by the default query function
	return func(ctx context.Context, _ dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
		switch {
		case setName.SetIdentifier == SetIdentifierProxied:
			return h.queryDNS(ctx, zone, setName, recordType)
		case setName.SetIdentifier != "":
			return nil, fmt.Errorf("unsupported set identifier: %s", setName.SetIdentifier)
		default:
			queryResult := defaultQueryFunc.Query(ctx, setName, recordType)
			return queryResult.RecordSet, queryResult.Err
		}
	}, nil
}

// queryDNS queries the DNS provider for the given DNS name and record type.
func (h *handler) queryDNS(ctx context.Context, zone dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
	domainName := dns.NormalizeDomainName(setName.DNSName)
	rl, policies, err := h.access.GetRecordList(ctx, domainName, string(recordType), zone)
	if err != nil {
		return nil, fmt.Errorf("queryDNS failed: %w", err)
	}
	if len(rl) == 0 {
		return nil, nil
	}
	var (
		records []*dns.Record
		ttl     int64
		policy  *dns.RoutingPolicy
	)
	for i, r := range rl {
		if r.GetSetIdentifier() != setName.SetIdentifier {
			continue
		}
		ttl = r.GetTTL()
		policy = policies[i]
		records = append(records, &dns.Record{
			Value: r.GetValue(),
		})
	}
	if len(records) == 0 {
		return nil, nil
	}

	return &dns.RecordSet{
		Type:          recordType,
		TTL:           ttl,
		Records:       records,
		RoutingPolicy: policy,
	}, nil
}

func checkValidRoutingPolicy(name dns.DNSSetName, req *provider.ChangeRequestUpdate) error {
	if req.Old != nil {
		if err := checkRoutingPolicyForDNSSet(name, req.Old); err != nil {
			return err
		}
	}
	if req.New != nil {
		if err := checkRoutingPolicyForDNSSet(name, req.New); err != nil {
			return err
		}
	}
	return nil
}

func checkRoutingPolicyForDNSSet(name dns.DNSSetName, rs *dns.RecordSet) error {
	if name.SetIdentifier == "" && rs.RoutingPolicy == nil {
		return nil
	}
	if name.SetIdentifier == "" {
		return fmt.Errorf("missing set identifier")
	}
	if rs.RoutingPolicy == nil {
		return fmt.Errorf("missing routing policy")
	}
	if rs.RoutingPolicy.Type != dns.RoutingPolicyProxied {
		return fmt.Errorf("unsupported routing policy: %s (supported is %s)", rs.RoutingPolicy.Type, dns.RoutingPolicyProxied)
	}
	if name.SetIdentifier != SetIdentifierProxied {
		return fmt.Errorf("unsupported set identifier: %s (supported is %s)", name.SetIdentifier, SetIdentifierProxied)
	}
	return nil
}

func (h *handler) ExecuteRequests(ctx context.Context, zone provider.DNSHostedZone, reqs provider.ChangeRequests) error {
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return err
	}

	return raw.ExecuteRequests(ctx, log, h.access, zone, reqs, checkValidRoutingPolicy)
}
