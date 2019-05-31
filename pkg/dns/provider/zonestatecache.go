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

type ZoneStateCache interface {
	GetZoneState(zone DNSHostedZone) DNSZoneState
	SetZoneState(zone DNSHostedZone, newState DNSZoneState)
	ExecuteRequests(zone DNSHostedZone, reqs []*ChangeRequest)
	DeleteZoneState(zone DNSHostedZone)
	RestrictCacheToZones(zones DNSHostedZones)
}

type nullZoneStateCache struct {
}

var _ ZoneStateCache = &nullZoneStateCache{}

func (c *nullZoneStateCache) GetZoneState(zone DNSHostedZone) DNSZoneState {
	return nil
}

func (c *nullZoneStateCache) SetZoneState(zone DNSHostedZone, newState DNSZoneState) {
}

func (c *nullZoneStateCache) DeleteZoneState(zone DNSHostedZone) {
}

func (c *nullZoneStateCache) ExecuteRequests(zone DNSHostedZone, reqs []*ChangeRequest) {
}

func (c *nullZoneStateCache) RestrictCacheToZones(zones DNSHostedZones) {
}

type zoneStateCache struct {
	lock       sync.Mutex
	inMemory   *InMemory
	lastSync   map[string]time.Time
	syncPeriod time.Duration
}

var _ ZoneStateCache = &zoneStateCache{}

func NewZoneStateCache(syncPeriod time.Duration) *zoneStateCache {
	return &zoneStateCache{inMemory: NewInMemory(), syncPeriod: syncPeriod, lastSync: map[string]time.Time{}}
}

func (c *zoneStateCache) GetZoneState(zone DNSHostedZone) DNSZoneState {
	c.lock.Lock()
	defer c.lock.Unlock()

	last, ok := c.lastSync[zone.Id()]
	if !ok {
		return nil
	}
	if last.Add(c.syncPeriod).Before(time.Now()) {
		c.deleteZoneState(zone)
		return nil
	}
	state, _ := c.inMemory.CloneZoneState(zone)
	return state
}

func (c *zoneStateCache) SetZoneState(zone DNSHostedZone, newState DNSZoneState) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.lastSync[zone.Id()] = time.Now()
	c.inMemory.SetZone(zone, newState)
}

func (c *zoneStateCache) DeleteZoneState(zone DNSHostedZone) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.deleteZoneState(zone)
}

func (c *zoneStateCache) deleteZoneState(zone DNSHostedZone) {
	delete(c.lastSync, zone.Id())
	c.inMemory.DeleteZone(zone)
}

func (c *zoneStateCache) ExecuteRequests(zone DNSHostedZone, reqs []*ChangeRequest) {
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

func (c *zoneStateCache) RestrictCacheToZones(zones DNSHostedZones) {
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
