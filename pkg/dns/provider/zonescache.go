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

type ZonesCacheUpdater func() (DNSHostedZones, error)

type ZonesCache interface {
	GetZones(updater ZonesCacheUpdater) (zones DNSHostedZones, cached bool, err error)
}

func NewZonesCache(ttl time.Duration) ZonesCache {
	if ttl <= 0 {
		return &nullZonesCache{}
	}
	return &stdZonesCache{ttl: ttl}
}

type nullZonesCache struct {
}

var _ ZonesCache = &nullZonesCache{}

func (c *nullZonesCache) GetZones(updater ZonesCacheUpdater) (DNSHostedZones, bool, error) {
	zones, err := updater()
	return zones, false, err
}

type stdZonesCache struct {
	lock  sync.Mutex
	ttl   time.Duration
	next  time.Time
	zones DNSHostedZones
	err   error
}

var _ ZonesCache = &stdZonesCache{}

func (c *stdZonesCache) GetZones(updater ZonesCacheUpdater) (DNSHostedZones, bool, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	useCache := time.Now().Before(c.next)
	if !useCache {
		c.zones, c.err = updater()
		updateTime := time.Now()
		if c.err != nil {
			c.next = updateTime.Add(c.ttl / 2)
		} else {
			c.next = updateTime.Add(c.ttl)
		}
	}
	return c.zones, useCache, c.err
}
