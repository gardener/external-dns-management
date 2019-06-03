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

package provider

import (
	"sync"
	"time"
)

type ZoneStateCacheUpdater func(zone DNSHostedZone) (DNSZoneState, error)

type ZoneStateCache interface {
	GetZoneState(zone DNSHostedZone, updater ZoneStateCacheUpdater) (state DNSZoneState, cached bool, err error)
	ExecuteRequests(zone DNSHostedZone, reqs []*ChangeRequest)
	DeleteZoneState(zone DNSHostedZone)
	RestrictCacheToZones(zones DNSHostedZones)
}

type nullZoneStateCache struct {
}

var _ ZoneStateCache = &nullZoneStateCache{}

func (c *nullZoneStateCache) GetZoneState(zone DNSHostedZone, updater ZoneStateCacheUpdater) (DNSZoneState, bool, error) {
	state, err := updater(zone)
	return state, false, err
}

func (c *nullZoneStateCache) DeleteZoneState(zone DNSHostedZone) {
}

func (c *nullZoneStateCache) ExecuteRequests(zone DNSHostedZone, reqs []*ChangeRequest) {
}

func (c *nullZoneStateCache) RestrictCacheToZones(zones DNSHostedZones) {
}

type defaultZoneStateCache struct {
	lock       sync.Mutex
	inMemory   *InMemory
	nextSync   map[string]time.Time
	syncPeriod time.Duration
}

var _ ZoneStateCache = &defaultZoneStateCache{}

func NewZoneStateCache(syncPeriod time.Duration) *defaultZoneStateCache {
	return &defaultZoneStateCache{inMemory: NewInMemory(), syncPeriod: syncPeriod, nextSync: map[string]time.Time{}}
}

func (c *defaultZoneStateCache) GetZoneState(zone DNSHostedZone, updater ZoneStateCacheUpdater) (DNSZoneState, bool, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	next, ok := c.nextSync[zone.Id()]
	if !ok || time.Now().After(next) {
		state, err := updater(zone)
		if err == nil {
			c.nextSync[zone.Id()] = time.Now().Add(c.syncPeriod)
			c.inMemory.SetZone(zone, state)
		} else {
			c.deleteZoneState(zone)
		}
		return state, false, err
	}
	state, _ := c.inMemory.CloneZoneState(zone)
	return state, true, nil
}

func (c *defaultZoneStateCache) DeleteZoneState(zone DNSHostedZone) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.deleteZoneState(zone)
}

func (c *defaultZoneStateCache) deleteZoneState(zone DNSHostedZone) {
	delete(c.nextSync, zone.Id())
	c.inMemory.DeleteZone(zone)
}

func (c *defaultZoneStateCache) ExecuteRequests(zone DNSHostedZone, reqs []*ChangeRequest) {
	c.lock.Lock()
	defer c.lock.Unlock()

	var err error
	nullMetrics := &NullMetrics{}
	for _, req := range reqs {
		err = c.inMemory.Apply(zone.Id(), req, nullMetrics)
		if err != nil {
			break
		}
	}

	if err != nil {
		c.deleteZoneState(zone)
	}
}

func (c *defaultZoneStateCache) RestrictCacheToZones(zones DNSHostedZones) {
	c.lock.Lock()
	defer c.lock.Unlock()

	obsoleteZoneIds := map[string]DNSHostedZone{}
	for _, zone := range c.inMemory.GetZones() {
		obsoleteZoneIds[zone.Id()] = zone
	}

	for _, zone := range zones {
		delete(obsoleteZoneIds, zone.Id())
	}

	for _, zone := range obsoleteZoneIds {
		c.deleteZoneState(zone)
	}
}
