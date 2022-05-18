/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package remote

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	"github.com/gardener/external-dns-management/pkg/server/remote/common"
)

type zoneid = string

type namespaceState struct {
	lock     sync.Mutex
	name     string
	handlers map[string]*handlerState
	tokens   map[string]*tokenState
	zones    map[zoneid]zonehandler
}

type zonehandler struct {
	zone    provider.DNSHostedZone
	handler *handlerState
}

type handlerState struct {
	lock    *dnsutils.TryLock
	name    string
	handler provider.LightDNSHandler
	zones   atomic.Value
}

type tokenState struct {
	clientID   string
	validUntil time.Time
}

func newNamespaceState(namespace string) *namespaceState {
	return &namespaceState{
		name:     namespace,
		handlers: map[string]*handlerState{},
		tokens:   map[string]*tokenState{},
	}
}

func (s *namespaceState) updateHandler(logger logger.LogContext, name string, handler provider.LightDNSHandler) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	mod := false
	zones, err := handler.GetZones()
	if err != nil {
		logger.Errorf("handler.GetZones failed: %w", err)
	}
	hstate := s.handlers[name]
	if hstate != nil {
		oldZones := hstate.getCachedZones()
		hstate.lock.Lock()
		hstate.handler = handler
		hstate.zones.Store(zones)
		hstate.lock.Unlock()
		mod = !zones.EquivalentTo(oldZones)
	} else {
		ctx := context.TODO()
		hstate = &handlerState{
			lock:    dnsutils.NewTryLock(ctx),
			name:    name,
			handler: handler,
			zones:   atomic.Value{},
		}
		hstate.zones.Store(zones)
		s.handlers[name] = hstate
		mod = true
	}

	if mod {
		s._refreshZones()
	}

	return mod
}

func (s *namespaceState) removeHandler(name string) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	_, exists := s.handlers[name]
	if exists {
		delete(s.handlers, name)
		s._refreshZones()
	}
	return exists
}

func (s *namespaceState) _refreshZones() {
	s.zones = map[zoneid]zonehandler{}
	for _, hstate := range s.handlers {
		zones := hstate.getCachedZones()
		for _, zone := range zones {
			s.zones[zone.Id().ID] = zonehandler{
				zone:    zone,
				handler: hstate,
			}
		}
	}
}

func (s *namespaceState) getToken(token string) (string, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	tstate := s.tokens[token]
	if tstate == nil || time.Now().After(tstate.validUntil) {
		if tstate != nil {
			delete(s.tokens, token)
		}
		return "", fmt.Errorf("%s for namespace %s", common.InvalidToken, s.name)
	}
	return tstate.clientID, nil
}

func (s *namespaceState) generateAndAddToken(tokenTTL time.Duration, rnd, clientID, server string) string {
	s.lock.Lock()
	defer s.lock.Unlock()

	validUntil := time.Now().Add(tokenTTL).UTC()
	token := fmt.Sprintf("%s|%s|%s|%s|%s", s.name, clientID, validUntil.Format(time.RFC3339), server, rnd)
	s.tokens[token] = &tokenState{
		clientID:   clientID,
		validUntil: validUntil,
	}
	return token
}

func (s *namespaceState) cleanupTokens(now time.Time) int {
	s.lock.Lock()
	defer s.lock.Unlock()

	count := 0
	for token, data := range s.tokens {
		if data.validUntil.Before(now) {
			delete(s.tokens, token)
			count++
		}
	}
	return count
}

func (s *namespaceState) getAllZones(spinning time.Duration) ([]provider.DNSHostedZone, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	allZones := []provider.DNSHostedZone{}
	for _, hstate := range s.handlers {
		zones, err := hstate.getZones(spinning)
		if err != nil {
			return nil, err
		}
		if zones != nil {
			allZones = append(allZones, zones...)
		}
	}
	s._refreshZones()

	return allZones, nil
}

func (s *namespaceState) lockupZone(spinning time.Duration, zoneid string) (*handlerState, provider.DNSHostedZone, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	zonehandler, ok := s.zones[zoneid]
	if !ok {
		return nil, nil, fmt.Errorf("zone %s not found", zoneid)
	}
	return zonehandler.handler, zonehandler.zone, nil
}

func (h *handlerState) getCachedZones() []provider.DNSHostedZone {
	v := h.zones.Load()
	if v == nil {
		return nil
	}
	return v.(provider.DNSHostedZones)
}

func (h *handlerState) getZones(spinning time.Duration) ([]provider.DNSHostedZone, error) {
	if !h.lock.TryLockSpinning(spinning) {
		return nil, fmt.Errorf("busy")
	}
	defer h.lock.Unlock()

	zones, err := h.handler.GetZones()
	h.zones.Store(zones)
	return zones, err
}
