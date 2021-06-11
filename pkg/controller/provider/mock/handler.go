/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. h file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package mock

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Handler struct {
	provider.DefaultDNSHandler
	config      provider.DNSHandlerConfig
	cache       provider.ZoneCache
	ctx         context.Context
	mock        *provider.InMemory
	mockConfig  MockConfig
	rateLimiter flowcontrol.RateLimiter
}

type MockConfig struct {
	Zones           []string `json:"zones"`
	FailGetZones    bool     `json:"failGetZones"`
	FailDeleteEntry bool     `json:"failDeleteEntry"`
}

var _ provider.DNSHandler = &Handler{}

// TestMock allows tests to access mocked DNSHosted Zones
var TestMock *provider.InMemory

func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	mock := provider.NewInMemory()
	TestMock = mock

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

	for _, dnsName := range h.mockConfig.Zones {
		if dnsName != "" {
			logger.Infof("Providing mock DNSZone %s", dnsName)
			hostedZone := provider.NewDNSHostedZone(h.ProviderType(), dnsName, dnsName, "", []string{}, false)
			mock.AddZone(hostedZone)
		}
	}

	h.cache, err = provider.NewZoneCache(config.CacheConfig, config.Metrics, nil, h.getZones, h.getZoneState)
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

func (h *Handler) getZones(cache provider.ZoneCache) (provider.DNSHostedZones, error) {
	if h.mockConfig.FailGetZones {
		return nil, fmt.Errorf("forced error by mockConfig.FailGetZones")
	}
	h.config.RateLimiter.Accept()
	zones := h.mock.GetZones()
	return zones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone, forceUpdate bool) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone, forceUpdate)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	h.config.RateLimiter.Accept()
	return h.mock.CloneZoneState(zone)
}

func (h *Handler) ReportZoneStateConflict(zone provider.DNSHostedZone, err error) bool {
	return h.cache.ReportZoneStateConflict(zone, err)
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
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
