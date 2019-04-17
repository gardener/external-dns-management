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
	"net/http"

	"github.com/gardener/controller-manager-library/pkg/logger"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Handler struct {
	config  provider.DNSHandlerConfig
	ctx     context.Context
	metrics provider.Metrics
	mock    *InMemory
}

type MockConfig struct {
	Zones    []string `json:"zones"`
	HttpPort string   `json:"httpPort,omitempty"`
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(logger logger.LogContext, config *provider.DNSHandlerConfig, metrics provider.Metrics) (provider.DNSHandler, error) {
	mock := NewInMemory()

	h := &Handler{
		config:  *config,
		metrics: metrics,
		mock:    mock,
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

	if mockConfig.HttpPort != "" {
		logger.Infof("Running mock dump service at port %s", mockConfig.HttpPort)
		go http.ListenAndServe(":"+mockConfig.HttpPort, mock)
	}

	return h, nil
}

func (h *Handler) ProviderType() string {
	return TYPE_CODE
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	zones := h.mock.GetZones()

	return zones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	dnssets, err := h.mock.CloneDNSSets(zone)
	if err != nil {
		return nil, err
	}
	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
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
	}

	return nil
}
