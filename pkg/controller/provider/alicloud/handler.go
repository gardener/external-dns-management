/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package alicloud

import (
	"fmt"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig
	cache  provider.ZoneCache
	access Access
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(logger logger.LogContext, config *provider.DNSHandlerConfig, metrics provider.Metrics) (provider.DNSHandler, error) {
	var err error

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *config,
	}

	accessKeyID := h.config.Properties["ACCESS_KEY_ID"]
	if accessKeyID == "" {
		accessKeyID = h.config.Properties["accessKeyID"]
	}
	if accessKeyID == "" {
		return nil, fmt.Errorf("'ACCESS_KEY_ID' or 'accessKeyID' required in secret")
	}
	accessKeySecret := h.config.Properties["ACCESS_KEY_SECRET"]
	if accessKeySecret == "" {
		accessKeySecret = h.config.Properties["accessKeySecret"]
	}
	if accessKeySecret == "" {
		return nil, fmt.Errorf("'ACCESS_KEY_SECRET' or 'accessKeySecret' required in secret")
	}

	access, err := NewAccess(accessKeyID, accessKeySecret, metrics)
	if err != nil {
		return nil, err
	}

	h.access = access

	h.cache, err = provider.NewZoneCache(*config.CacheConfig.CopyWithDisabledZoneStateCache(), metrics, nil)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (h *Handler) ProviderType() string {
	return TYPE_CODE
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones(h.getZones)
}

func (h *Handler) getZones(data interface{}) (provider.DNSHostedZones, error) {
	raw := []alidns.Domain{}
	{
		f := func(zone alidns.Domain) (bool, error) {
			raw = append(raw, zone)
			return true, nil
		}
		err := h.access.ListDomains(f)
		if err != nil {
			return nil, err
		}
	}

	zones := provider.DNSHostedZones{}
	{
		for _, z := range raw {
			forwarded := []string{}
			f := func(r alidns.Record) (bool, error) {
				if r.Type == dns.RS_NS {
					name := GetDNSName(r)
					if name != z.DomainName {
						forwarded = append(forwarded, name)
					}
				}
				return true, nil
			}
			err := h.access.ListRecords(z.DomainName, f)
			if err != nil {
				if checkAccessForbidden(err) {
					// It is reasonable for some RAM user, it is only allowed to access certain domain's records detail
					// As a result, h domain should not be appended to the hosted zones
					continue
				}
				return nil, err
			}
			hostedZone := provider.NewDNSHostedZone(
				h.ProviderType(), z.DomainId,
				z.DomainName, z.DomainName, forwarded)
			zones = append(zones, hostedZone)
		}
	}

	return zones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone, h.getZoneState)
}

func (h *Handler) getZoneState(data interface{}, zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	state := newState()

	f := func(r alidns.Record) (bool, error) {
		state.addRecord(r)
		return true, nil
	}
	err := h.access.ListRecords(zone.Key(), f)
	if err != nil {
		return nil, err
	}
	state.calculateDNSSets()
	return state, nil
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	if err == nil {
		h.cache.ExecuteRequests(zone, reqs)
	} else {
		h.cache.DeleteZoneState(zone)
	}
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h, state.(*zonestate), zone)
	for _, r := range reqs {
		exec.addChange(r)
	}
	if h.config.DryRun {
		logger.Infof("no changes in dryrun mode for AliCloud")
		return nil
	}
	return exec.submitChanges()
}

func checkAccessForbidden(err error) bool {
	if err != nil {
		switch err.(type) {
		case *errors.ServerError:
			serverErr := err.(*errors.ServerError)
			if serverErr.ErrorCode() == "Forbidden.RAM" {
				return true
			}
		}
	}

	return false
}
