// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package alicloud

import (
	"context"
	"fmt"

	alidns "github.com/alibabacloud-go/alidns-20150109/v5/client"
	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"

	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
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

// NewHandler creates a new DNS handler for the Alicloud DNS provider.
func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	var err error

	h := &handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *c,
	}

	accessKeyID, err := c.GetRequiredProperty("ACCESS_KEY_ID", "accessKeyID")
	if err != nil {
		return nil, err
	}
	accessKeySecret, err := c.GetRequiredProperty("ACCESS_KEY_SECRET", "accessKeySecret")
	if err != nil {
		return nil, err
	}

	access, err := newAccess(accessKeyID, accessKeySecret, c.Metrics, c.RateLimiter)
	if err != nil {
		return nil, perrs.WrapAsHandlerError(err, "creating alicloud access with client credentials failed")
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

	rawZones := []*alidns.DescribeDomainsResponseBodyDomainsDomain{}
	{
		f := func(zone *alidns.DescribeDomainsResponseBodyDomainsDomain) (bool, error) {
			domainID := ptr.Deref(zone.DomainId, "")
			if h.isBlockedZone(domainID) {
				log.Info("ignoring blocked zone", "zone", domainID)
			} else {
				rawZones = append(rawZones, zone)
			}
			return true, nil
		}
		err := h.access.ListDomains(f)
		if err != nil {
			return nil, perrs.WrapAsHandlerError(err, "list domains failed")
		}
	}

	var zones []provider.DNSHostedZone
	for _, z := range rawZones {
		domainID := ptr.Deref(z.DomainId, "")
		domainName := ptr.Deref(z.DomainName, "")
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), domainID, domainName, domainName, false)
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
	for _, zone := range h.getAdvancedOptions().BlockedZones {
		if zone == zoneID {
			return true
		}
	}
	return false
}

// GetCustomQueryDNSFunc returns a custom DNS query function for the Alicloud DNS provider.
func (h *handler) GetCustomQueryDNSFunc(_ dns.ZoneInfo, factory utils.QueryDNSFactoryFunc) (provider.CustomQueryDNSFunc, error) {
	defaultQueryFunc, err := factory()
	if err != nil {
		return nil, fmt.Errorf("failed to create default query function: %w", err)
	}
	return func(ctx context.Context, zone dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
		switch {
		case setName.SetIdentifier != "":
			// routing policies with set identifiers are not supported by the default query function
			return h.queryDNS(ctx, zone, setName, recordType)
		default:
			// For all other record types, we can use the default query function
			queryResult := defaultQueryFunc.Query(ctx, setName, recordType)
			return queryResult.RecordSet, queryResult.Err
		}
	}, nil
}

func (h *handler) queryDNS(ctx context.Context, zone dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
	rl, policies, err := h.access.GetRecordList(ctx, setName.DNSName, string(recordType), zone)
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

func (h *handler) ExecuteRequests(ctx context.Context, zone provider.DNSHostedZone, reqs provider.ChangeRequests) error {
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return err
	}

	return raw.ExecuteRequests(ctx, log, h.access, zone, reqs, checkValidRoutingPolicy)
}
