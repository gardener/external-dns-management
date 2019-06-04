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
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"os"
	"os/signal"
	"sync"
	"time"
)

type ZoneCacheConfig struct {
	persistDir            string
	zonesTTL              time.Duration
	stateTTL              time.Duration
	disableZoneStateCache bool
}

func (c *ZoneCacheConfig) CopyWithDisabledZoneStateCache() *ZoneCacheConfig {
	return &ZoneCacheConfig{persistDir: c.persistDir, zonesTTL: c.zonesTTL, stateTTL: c.stateTTL, disableZoneStateCache: true}
}

type ZoneCacheZoneUpdater func(providerData interface{}) (DNSHostedZones, error)

type ZoneCacheStateUpdater func(providerData interface{}, zone DNSHostedZone) (DNSZoneState, error)

type ZoneCache interface {
	GetZones(updater ZoneCacheZoneUpdater) (DNSHostedZones, error)
	GetZoneState(zone DNSHostedZone, updater ZoneCacheStateUpdater) (DNSZoneState, error)
	DeleteZoneState(zone DNSHostedZone)
	ExecuteRequests(zone DNSHostedZone, reqs []*ChangeRequest)
	GetProviderData() interface{}
}

func NewZoneCache(config ZoneCacheConfig, metrics Metrics, providerData interface{}) (ZoneCache, error) {
	if config.disableZoneStateCache {
		return &onlyZonesCache{config: config, providerData: providerData}, nil
	} else {
		if config.persistDir != "" {
			err := os.MkdirAll(config.persistDir, 0777)
			if err != nil {
				return nil, fmt.Errorf("Creating persistent directory for zone cache at %s failed with %s", config.persistDir, err)
			}
		}
		state := &zoneState{inMemory: NewInMemory(), ttl: config.stateTTL, next: map[string]time.Time{}}
		cache := &defaultZoneCache{config: config, metrics: metrics, providerData: providerData, state: state}
		if config.persistDir != "" {
			err := cache.restoreFromDisk()
			if err != nil {
				return nil, fmt.Errorf("Restoring zone cache from persistent directory %s failed with %s", config.persistDir, err)
			}
		}
		cache.persistChan = make(chan string)
		go cache.backgroundWriter()
		return cache, nil
	}
}

type onlyZonesCache struct {
	lock         sync.Mutex
	config       ZoneCacheConfig
	zones        DNSHostedZones
	zonesErr     error
	zonesNext    time.Time
	providerData interface{}
}

var _ ZoneCache = &onlyZonesCache{}

func (c *onlyZonesCache) GetZones(updater ZoneCacheZoneUpdater) (DNSHostedZones, error) {
	zones, err := updater(c.providerData)
	return zones, err
}

func (c *onlyZonesCache) GetZoneState(zone DNSHostedZone, updater ZoneCacheStateUpdater) (DNSZoneState, error) {
	state, err := updater(c.providerData, zone)
	return state, err
}

func (c *onlyZonesCache) DeleteZoneState(zone DNSHostedZone) {
}

func (c *onlyZonesCache) ExecuteRequests(zone DNSHostedZone, reqs []*ChangeRequest) {
}

func (c *onlyZonesCache) GetProviderData() interface{} {
	return c.providerData
}

type defaultZoneCache struct {
	lock         sync.Mutex
	config       ZoneCacheConfig
	metrics      Metrics
	zones        DNSHostedZones
	zonesErr     error
	zonesNext    time.Time
	state        *zoneState
	providerData interface{}
	persistChan  chan string
}

var _ ZoneCache = &defaultZoneCache{}

func (c *defaultZoneCache) GetZones(updater ZoneCacheZoneUpdater) (DNSHostedZones, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if time.Now().After(c.zonesNext) {
		c.zones, c.zonesErr = updater(c.providerData)
		updateTime := time.Now()
		if c.zonesErr != nil {
			c.zonesNext = updateTime.Add(c.config.zonesTTL / 2)
		} else {
			c.zonesNext = updateTime.Add(c.config.zonesTTL)
		}
		c.state.RestrictCacheToZones(c.zones)
	} else {
		c.metrics.AddRequests(M_CACHED_GETZONES, 1)
	}
	return c.zones, c.zonesErr
}

func (c *defaultZoneCache) GetZoneState(zone DNSHostedZone, updater ZoneCacheStateUpdater) (DNSZoneState, error) {
	state, cached, err := c.state.GetZoneState(zone, c.providerData, updater)
	if cached {
		c.metrics.AddRequests(M_CACHED_GETZONESTATE, 1)
	} else {
		c.persistChan <- zone.Id()
	}
	return state, err
}

func (c *defaultZoneCache) DeleteZoneState(zone DNSHostedZone) {
	c.state.DeleteZoneState(zone)
	c.persistChan <- zone.Id()
}

func (c *defaultZoneCache) ExecuteRequests(zone DNSHostedZone, reqs []*ChangeRequest) {
	c.state.ExecuteRequests(zone, reqs)
	c.persistChan <- zone.Id()
}

func (c *defaultZoneCache) GetProviderData() interface{} {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.providerData
}

func (c *defaultZoneCache) restoreFromDisk() error {
	// TODO
	return nil
}

func (c *defaultZoneCache) backgroundWriter() {
	ticker := time.NewTicker(3 * time.Second)
	chSignal := make(chan os.Signal)
	signal.Notify(chSignal, os.Interrupt, os.Kill)

	outstandingZones := map[string]bool{}
	for {
		select {
		case zoneId := <-c.persistChan:
			outstandingZones[zoneId] = true
		case <-ticker.C:
			if len(outstandingZones) > 0 {
				zonesToWrite := outstandingZones
				outstandingZones = map[string]bool{}
				go c.writeZones(zonesToWrite)
			}
		case <-chSignal:
			if len(outstandingZones) > 0 {
				go c.writeZones(outstandingZones)
			}
			return
		}
	}
}

func (c *defaultZoneCache) writeZones(zoneids map[string]bool) {
	logger.Infof("Writing zone cache for %v", zoneids)
}

type zoneState struct {
	lock     sync.Mutex
	ttl      time.Duration
	inMemory *InMemory
	next     map[string]time.Time
}

func (s *zoneState) GetZoneState(zone DNSHostedZone, data interface{}, updater ZoneCacheStateUpdater) (DNSZoneState, bool, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	next, ok := s.next[zone.Id()]
	if !ok || time.Now().After(next) {
		state, err := updater(data, zone)
		if err == nil {
			s.next[zone.Id()] = time.Now().Add(s.ttl)
			s.inMemory.SetZone(zone, state)
		} else {
			s.deleteZoneState(zone)
		}
		return state, false, err
	}

	state, _ := s.inMemory.CloneZoneState(zone)
	return state, true, nil
}

func (s *zoneState) ExecuteRequests(zone DNSHostedZone, reqs []*ChangeRequest) {
	s.lock.Lock()
	defer s.lock.Unlock()

	var err error
	nullMetrics := &NullMetrics{}
	for _, req := range reqs {
		err = s.inMemory.Apply(zone.Id(), req, nullMetrics)
		if err != nil {
			break
		}
	}

	if err != nil {
		s.deleteZoneState(zone)
	}
}

func (s *zoneState) DeleteZoneState(zone DNSHostedZone) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.deleteZoneState(zone)
}

func (s *zoneState) deleteZoneState(zone DNSHostedZone) {
	delete(s.next, zone.Id())
	s.inMemory.DeleteZone(zone)
}

func (s *zoneState) RestrictCacheToZones(zones DNSHostedZones) {
	s.lock.Lock()
	defer s.lock.Unlock()

	obsoleteZoneIds := map[string]DNSHostedZone{}
	for _, zone := range s.inMemory.GetZones() {
		obsoleteZoneIds[zone.Id()] = zone
	}

	for _, zone := range zones {
		delete(obsoleteZoneIds, zone.Id())
	}

	for _, zone := range obsoleteZoneIds {
		s.deleteZoneState(zone)
	}
}
