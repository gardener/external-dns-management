/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. h file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use h file except in compliance with the License.
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

	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Handler struct {
	provider.DefaultDNSHandler
	config  provider.DNSHandlerConfig
	cache   provider.ZoneCache
	ctx     context.Context
	metrics provider.Metrics
	mock    *provider.InMemory
}

type MockConfig struct {
	Zones []string `json:"zones"`
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(logger logger.LogContext, config *provider.DNSHandlerConfig, metrics provider.Metrics) (provider.DNSHandler, error) {
	mock := provider.NewInMemory()

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *config,
		metrics:           metrics,
		mock:              mock,
	}

	mockConfig := MockConfig{}
	err := json.Unmarshal(config.Config.Raw, &mockConfig)
	if err != nil {
		return nil, fmt.Errorf("unmarshal mock providerConfig failed with: %s", err)
	}

	for _, dnsName := range mockConfig.Zones {
		if dnsName != "" {
			logger.Infof("Providing mock DNSZone %s", dnsName)
			hostedZone := provider.NewDNSHostedZone(
				h.ProviderType(),
				dnsName,
				dnsName,
				"",
				[]string{},
			)
			mock.AddZone(hostedZone)
		}
	}

	h.cache, err = provider.NewZoneCache(config.CacheConfig, metrics, nil)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (h *Handler) Release() {
	h.cache.Release()
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones(h.getZones)
}

func (h *Handler) getZones() (provider.DNSHostedZones, error) {
	zones := h.mock.GetZones()
	return zones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone, h.getZoneState)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.mock.CloneZoneState(zone)
}

func (this *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := this.executeRequests(logger, zone, state, reqs)
	if err == nil {
		this.cache.ExecuteRequests(zone, reqs)
	} else {
		this.cache.DeleteZoneState(zone)
	}
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	var succeeded, failed int
	for _, r := range reqs {
		err := h.mock.Apply(zone.Id(), r, h.metrics)
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
