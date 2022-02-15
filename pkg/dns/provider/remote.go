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
	"strings"
	"sync"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/external-dns-management/pkg/server/remote/embed"
	corev1 "k8s.io/api/core/v1"
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

func createRemoteAccessConfig(c controller.Interface) (*embed.RemoteAccessServerConfig, error) {
	remoteAccessPort, err := c.GetIntOption(OPT_REMOTE_ACCESS_PORT)
	if err != nil {
		return nil, err
	}
	if remoteAccessPort == 0 {
		return nil, nil
	}

	values := map[string]string{}
	for _, key := range []string{OPT_REMOTE_ACCESS_CACERT, OPT_REMOTE_ACCESS_SERVER_SECRET_NAME} {
		value, _ := c.GetStringOption(key)
		if value == "" {
			return nil, fmt.Errorf("missing %s for activated remote access server", key)
		}
		values[key] = value
	}
	parts := strings.Split(values[OPT_REMOTE_ACCESS_SERVER_SECRET_NAME], "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid format for %s: expected '<namespace>/<name>'", OPT_REMOTE_ACCESS_SERVER_SECRET_NAME)
	}
	secretName := resources.NewObjectName(parts[0], parts[1])
	return &embed.RemoteAccessServerConfig{
		Port:                 remoteAccessPort,
		CACertFilename:       values[OPT_REMOTE_ACCESS_CACERT],
		SecretName:           secretName,
		ServerSecretProvider: &serverSecretProvider{},
	}, nil
}

type serverSecretProvider struct {
	lock     sync.Mutex
	handlers []embed.ServerSecretUpdateHandler
	secret   *corev1.Secret
}

var _ embed.ServerSecretProvider = &serverSecretProvider{}

func (s *serverSecretProvider) UpdateSecret(secret *corev1.Secret) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.secret = secret
	for _, handler := range s.handlers {
		handler(secret)
	}
}

func (s *serverSecretProvider) AddUpdateHandler(handler embed.ServerSecretUpdateHandler) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.handlers = append(s.handlers, handler)
}
