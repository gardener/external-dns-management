// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Handler struct {
	provider.DefaultDNSHandler
	config      provider.DNSHandlerConfig
	cache       provider.ZoneCache
	mock        *provider.InMemory
	mockConfig  MockConfig
	rateLimiter flowcontrol.RateLimiter
}

type MockZone struct {
	ZonePrefix string `json:"zonePrefix"`
	DNSName    string `json:"dnsName"`
}

func (m MockZone) ZoneID() dns.ZoneID {
	return dns.NewZoneID(TYPE_CODE, m.ZonePrefix+m.DNSName)
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
var TestMock = map[string]*provider.InMemory{}

func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	mock := provider.NewInMemory()

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
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
			logger.Infof("Providing mock DNSZone %s[%s]", mockZone.DNSName, zoneID)
			isPrivate := strings.Contains(mockZone.ZonePrefix, ":private:")
			hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zoneID, mockZone.DNSName, "", isPrivate)
			mock.AddZone(hostedZone)
		}
	}

	h.cache, err = config.ZoneCacheFactory.CreateZoneCache(provider.CacheZoneState, config.Metrics, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (h *Handler) Release() {
	h.cache.Release()
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) getZones(_ provider.ZoneCache) (provider.DNSHostedZones, error) {
	if h.mockConfig.FailGetZones {
		return nil, fmt.Errorf("forced error by mockConfig.FailGetZones")
	}
	h.config.RateLimiter.Accept()
	zones := h.mock.GetZones()
	return zones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, _ provider.ZoneCache) (provider.DNSZoneState, error) {
	h.config.RateLimiter.Accept()
	return h.mock.CloneZoneState(zone)
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	if h.mockConfig.LatencyMillis > 0 {
		time.Sleep(time.Duration(h.mockConfig.LatencyMillis) * time.Millisecond)
	}
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, _ provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	var succeeded, failed int
	for _, r := range reqs {
		h.config.RateLimiter.Accept()
		var err error
		if h.mockConfig.FailDeleteEntry && r.Action == provider.R_DELETE {
			err = fmt.Errorf("forced error by mockConfig.FailDeleteEntry")
		} else {
			err = h.mock.Apply(zone.Id(), r, h.config.Metrics)
		}
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
	if succeeded > 0 {
		logger.Infof("Succeeded updates for records in zone %s: %d", zone.Id(), succeeded)
	}
	if failed > 0 {
		logger.Infof("Failed updates for records in zone %s: %d", zone.Id(), failed)
		return fmt.Errorf("%d changed failed", failed)
	}

	return nil
}
