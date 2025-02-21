// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

type Handler struct {
	provider.DefaultDNSHandler
	config      provider.DNSHandlerConfig
	mock        *InMemory
	mockConfig  MockConfig
	rateLimiter flowcontrol.RateLimiter
}

type MockZone struct {
	ZonePrefix string `json:"zonePrefix"`
	DNSName    string `json:"dnsName"`
}

func (m MockZone) ZoneID() dns.ZoneID {
	return dns.NewZoneID(ProviderType, m.ZonePrefix+m.DNSName)
}

type MockConfig struct {
	Name            string     `json:"name"`
	Zones           []MockZone `json:"zones"`
	FailGetZones    bool       `json:"failGetZones"`
	FailDeleteEntry bool       `json:"failDeleteEntry"`
	LatencyMillis   int        `json:"latencyMillis"`
}

var _ provider.DNSHandler = &Handler{}

// TestMock allows tests to access mocked DNSHosted Zones
var TestMock = map[string]*InMemory{}

func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	mock := NewInMemory()

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *config,
		mock:              mock,
		rateLimiter:       config.RateLimiter,
	}

	err := json.Unmarshal(config.Config.Raw, &h.mockConfig)
	if err != nil {
		return nil, fmt.Errorf("unmarshal mock providerConfig failed with: %s", err)
	}

	TestMock[h.mockConfig.Name] = mock

	for _, mockZone := range h.mockConfig.Zones {
		if mockZone.DNSName != "" {
			zoneID := mockZone.ZoneID().ID
			config.Log.Info("Providing mock DNSZone", "zone", mockZone.DNSName, "zoneID", zoneID)
			isPrivate := strings.Contains(mockZone.ZonePrefix, ":private:")
			hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zoneID, mockZone.DNSName, "", isPrivate)
			mock.AddZone(hostedZone)
		}
	}

	return h, nil
}

func (h *Handler) Release() {
}

func (h *Handler) GetZones(ctx context.Context) ([]provider.DNSHostedZone, error) {
	if h.mockConfig.FailGetZones {
		return nil, fmt.Errorf("forced error by mockConfig.FailGetZones")
	}
	h.config.RateLimiter.Accept()
	zones := h.mock.GetZones()
	return zones, nil
}

func (h *Handler) QueryDNS(_ context.Context, zone provider.DNSHostedZone, domainName string, recordType dns.RecordType) ([]dns.Record, int64, error) {
	result := h.mock.GetRecordset(zone.ZoneID(), dns.DNSSetName{DNSName: dns.NormalizeDomainName(domainName)}, recordType)
	if result == nil {
		return nil, 0, nil
	}
	records := make([]dns.Record, len(result.Records))
	for i, r := range result.Records {
		records[i] = dns.Record{Value: r.Value}
	}
	return records, result.TTL, nil
}

func (h *Handler) ExecuteRequests(ctx context.Context, zone provider.DNSHostedZone, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(ctx, zone, reqs)
	if h.mockConfig.LatencyMillis > 0 {
		time.Sleep(time.Duration(h.mockConfig.LatencyMillis) * time.Millisecond)
	}
	return err
}

func (h *Handler) executeRequests(ctx context.Context, zone provider.DNSHostedZone, reqs []*provider.ChangeRequest) error {
	log, err := logr.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to get logger from context: %w", err)
	}
	log.Info("Executing requests", "zoneID", zone.ZoneID(), "requests", len(reqs))
	return fmt.Errorf("not implemented")
	/*
		var succeeded, failed int
		for _, r := range reqs {
			h.config.RateLimiter.Accept()
			var err error
			if h.mockConfig.FailDeleteEntry && r.Action == provider.R_DELETE {
				err = fmt.Errorf("forced error by mockConfig.FailDeleteEntry")
			} else {
				err = h.mock.Apply(zone.ZoneID(), r, h.config.Metrics)
			}
			if err != nil {
				failed++
				log.Error(err, "Apply failed")
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
		if succeeded > 0 {
			log.Info("Succeeded updates for records in zone", "zoneID", zone.ZoneID(), "succeeded", succeeded)
		}
		if failed > 0 {
			log.Info("Failed updates for records in zone", "zoneID", zone.ZoneID(), "failed", failed)
			return fmt.Errorf("%d changed failed", failed)
		}

		return nil
	*/
}
