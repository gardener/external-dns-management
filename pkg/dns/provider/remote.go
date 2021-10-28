/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package provider

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
)

var _ LightDNSHandler = &dnsProviderVersionLightHandler{}

type dnsProviderVersionLightHandler struct {
	version *dnsProviderVersion
}

func handler(version *dnsProviderVersion) LightDNSHandler {
	return dnsProviderVersionLightHandler{
		version: version,
	}
}

func (h dnsProviderVersionLightHandler) ProviderType() string {
	return h.version.TypeCode()
}

func (h dnsProviderVersionLightHandler) GetZones() (DNSHostedZones, error) {
	return h.version.GetZones(), nil
}

func (h dnsProviderVersionLightHandler) GetZoneState(zone DNSHostedZone) (DNSZoneState, error) {
	for _, z := range h.version.GetZones() {
		if z.Id() == zone.Id() {
			return h.version.GetZoneState(zone)
		}
	}
	return nil, fmt.Errorf("zone %s is not included", zone.Id())
}

func (h dnsProviderVersionLightHandler) ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, reqs []*ChangeRequest) error {
	return h.version.ExecuteRequests(logger, zone, state, reqs)
}
