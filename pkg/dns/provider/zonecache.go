/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. h file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ZoneCacheConfig struct {
	context               context.Context
	logger                logger.LogContext
	persistDir            string
	zonesTTL              time.Duration
	stateTTL              time.Duration
	disableZoneStateCache bool
}

func (c *ZoneCacheConfig) CopyWithDisabledZoneStateCache() *ZoneCacheConfig {
	return &ZoneCacheConfig{context: c.context, logger: c.logger,
		persistDir: c.persistDir, zonesTTL: c.zonesTTL, stateTTL: c.stateTTL, disableZoneStateCache: true}
}

type ZoneCacheZoneUpdater func(cache ZoneCache) (DNSHostedZones, error)

type ZoneCacheStateUpdater func(zone DNSHostedZone, cache ZoneCache) (DNSZoneState, error)

type ZoneCache interface {
	GetZones() (DNSHostedZones, error)
	GetZoneState(zone DNSHostedZone) (DNSZoneState, error)
	ApplyRequests(err error, zone DNSHostedZone, reqs []*ChangeRequest)
	GetHandlerData() HandlerData
	Release()
}

type HandlerData interface {
	Marshal(zoneID string) (*PersistentHandlerData, error)
	Unmarshal(zoneID string, data *PersistentHandlerData) error
	DeleteZone(zoneID string)
}

func NewZoneCache(config ZoneCacheConfig, metrics Metrics, handlerData HandlerData,
	zonesUpdater ZoneCacheZoneUpdater, stateUpdater ZoneCacheStateUpdater) (ZoneCache, error) {
	common := abstractZonesCache{config: config, zonesUpdater: zonesUpdater, stateUpdater: stateUpdater}
	if config.disableZoneStateCache {
		cache := &onlyZonesCache{abstractZonesCache: common, handlerData: handlerData}
		return cache, nil
	} else {
		return newDefaultZoneCache(common, metrics, handlerData)
	}
}

type ForwardedDomainsHandlerData struct {
	lock             sync.Mutex
	forwardedDomains map[string][]string
}

func NewForwardedDomainsHandlerData() *ForwardedDomainsHandlerData {
	return &ForwardedDomainsHandlerData{forwardedDomains: map[string][]string{}}
}

var _ HandlerData = &ForwardedDomainsHandlerData{}

func (hd *ForwardedDomainsHandlerData) GetForwardedDomains(zoneid string) []string {
	hd.lock.Lock()
	defer hd.lock.Unlock()
	return hd.forwardedDomains[zoneid]
}

func (hd *ForwardedDomainsHandlerData) SetForwardedDomains(zoneid string, value []string) {
	hd.lock.Lock()
	defer hd.lock.Unlock()

	if value != nil {
		hd.forwardedDomains[zoneid] = value
	} else {
		delete(hd.forwardedDomains, zoneid)
	}
}

func (hd *ForwardedDomainsHandlerData) Marshal(zoneID string) (*PersistentHandlerData, error) {
	hd.lock.Lock()
	defer hd.lock.Unlock()

	data := hd.forwardedDomains[zoneID]
	if data == nil {
		return nil, nil
	}
	bytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &PersistentHandlerData{Name: "ForwardedDomains", Version: "1", Value: &runtime.RawExtension{Raw: bytes}}, nil
}

func (hd *ForwardedDomainsHandlerData) Unmarshal(zoneID string, data *PersistentHandlerData) error {
	hd.lock.Lock()
	defer hd.lock.Unlock()

	if data == nil {
		return nil
	}
	if data.Name != "ForwardedDomains" || data.Version != "1" {
		return fmt.Errorf("unexpected HandlerData: %s %s", data.Name, data.Version)
	}
	if data.Value == nil {
		return fmt.Errorf("missing value in HandlerData: %s %s", data.Name, data.Version)
	}

	var value []string
	err := json.Unmarshal([]byte(data.Value.Raw), &value)
	if err != nil {
		return err
	}
	hd.forwardedDomains[zoneID] = value
	return nil
}

func (hd *ForwardedDomainsHandlerData) DeleteZone(zoneID string) {
	hd.lock.Lock()
	defer hd.lock.Unlock()

	delete(hd.forwardedDomains, zoneID)
}

type abstractZonesCache struct {
	lock         sync.Mutex
	config       ZoneCacheConfig
	zones        DNSHostedZones
	zonesErr     error
	zonesNext    time.Time
	zonesUpdater ZoneCacheZoneUpdater
	stateUpdater ZoneCacheStateUpdater
}

type onlyZonesCache struct {
	abstractZonesCache
	handlerData HandlerData
}

var _ ZoneCache = &onlyZonesCache{}

func (c *onlyZonesCache) GetZones() (DNSHostedZones, error) {
	zones, err := c.zonesUpdater(c)
	return zones, err
}

func (c *onlyZonesCache) GetZoneState(zone DNSHostedZone) (DNSZoneState, error) {
	state, err := c.stateUpdater(zone, c)
	return state, err
}

func (c *onlyZonesCache) ApplyRequests(err error, zone DNSHostedZone, reqs []*ChangeRequest) {
}

func (c *onlyZonesCache) GetHandlerData() HandlerData {
	return c.handlerData
}

func (c *onlyZonesCache) Release() {
}

type defaultZoneCache struct {
	abstractZonesCache
	logger   logger.LogContext
	metrics  Metrics
	state    *zoneState
	persist  bool
	persistC chan string
}

var _ ZoneCache = &defaultZoneCache{}

func newDefaultZoneCache(common abstractZonesCache, metrics Metrics, handlerData HandlerData) (*defaultZoneCache, error) {
	state := &zoneState{
		inMemory:    NewInMemory(),
		ttl:         common.config.stateTTL,
		persistDir:  common.config.persistDir,
		next:        map[string]time.Time{},
		handlerData: handlerData,
	}
	persist := common.config.persistDir != ""
	cache := &defaultZoneCache{abstractZonesCache: common, logger: common.config.logger, metrics: metrics, state: state, persist: persist}
	if persist {
		err := os.MkdirAll(common.config.persistDir, 0777)
		if err != nil {
			return nil, fmt.Errorf("creating persistent directory for zone cache at %s failed with %s", common.config.persistDir, err)
		}

		err = cache.restoreFromDisk()
		if err != nil {
			return nil, fmt.Errorf("restoring zone cache from persistent directory %s failed with %s", common.config.persistDir, err)
		}
		cache.persistC = make(chan string)
		go cache.backgroundWriter()
	}
	return cache, nil
}

func (c *defaultZoneCache) GetZones() (DNSHostedZones, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if time.Now().After(c.zonesNext) {
		c.zones, c.zonesErr = c.zonesUpdater(c)
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

func (c *defaultZoneCache) GetZoneState(zone DNSHostedZone) (DNSZoneState, error) {
	state, cached, err := c.state.GetZoneState(zone, c)
	if cached {
		c.metrics.AddRequests(M_CACHED_GETZONESTATE, 1)
	} else {
		c.persistZone(zone)
	}
	return state, err
}

func (c *defaultZoneCache) persistZone(zone DNSHostedZone) {
	if c.persist {
		c.persistC <- zone.Id()
	}
}

func (c *defaultZoneCache) deleteZoneState(zone DNSHostedZone) {
	c.state.DeleteZoneState(zone)
	c.persistZone(zone)
}

func (c *defaultZoneCache) ApplyRequests(err error, zone DNSHostedZone, reqs []*ChangeRequest) {
	if err == nil {
		c.state.ExecuteRequests(zone, reqs)
		c.persistZone(zone)
	} else {
		c.deleteZoneState(zone)
	}
}

func (c *defaultZoneCache) GetHandlerData() HandlerData {
	return c.state.GetHandlerData()
}

func (c *defaultZoneCache) Release() {
	c.state.RestrictCacheToZones(DNSHostedZones{})
}

func (c *defaultZoneCache) backgroundWriter() {
	ticker := time.NewTicker(3 * time.Second)

	written := make(chan string)
	zonesToWrite := map[string]bool{}
	zonesWriting := map[string]bool{}
	for {
		select {
		case zoneid := <-c.persistC:
			zonesToWrite[zoneid] = true
		case zoneid := <-written:
			zonesWriting[zoneid] = false
		case <-c.config.context.Done():
			for zoneid := range zonesToWrite {
				if !zonesWriting[zoneid] {
					zonesWriting[zoneid] = true
					go c.writeZone(zoneid, written)
				}
			}
			ticker.Stop()
			return
		case <-ticker.C:
			for zoneid := range zonesToWrite {
				if !zonesWriting[zoneid] {
					zonesWriting[zoneid] = true
					go c.writeZone(zoneid, written)
					delete(zonesToWrite, zoneid)
				}
			}
		}
	}
}

type PersistentZone struct {
	ProviderType     string   `json:"providerType"`
	Key              string   `json:"key"`
	Id               string   `json:"id"`
	Domain           string   `json:"domain"`
	ForwardedDomains []string `json:"forwardedDomains"`
}

func (z *PersistentZone) ToDNSHostedZone() DNSHostedZone {
	return NewDNSHostedZone(z.ProviderType, z.Id, z.Domain, z.Key, z.ForwardedDomains)
}

func NewPersistentZone(zone DNSHostedZone) *PersistentZone {
	return &PersistentZone{
		ProviderType:     zone.ProviderType(),
		Id:               zone.Id(),
		Key:              zone.Key(),
		Domain:           zone.Domain(),
		ForwardedDomains: zone.ForwardedDomains(),
	}
}

type PersistentZoneState struct {
	Version     string                 `json:"version"`
	Valid       time.Time              `json:"valid"`
	Zone        PersistentZone         `json:"zone"`
	DNSSets     dns.DNSSets            `json:"dnssets,omitempty"`
	HandlerData *PersistentHandlerData `json:"handlerData,omitempty"`
}

type PersistentHandlerData struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Value   *runtime.RawExtension
}

const TempSuffix = ".~.tmp"

func (c *defaultZoneCache) writeZone(zoneid string, written chan<- string) {
	c.logger.Infof("writing zone cache for %s", zoneid)

	err := c.state.WriteZone(zoneid)
	if err != nil {
		c.logger.Warn(err)
	}
	written <- zoneid
}

var alreadyCleanedupOutdated int32 = 0

func zoneCacheCleanupOutdated(logger logger.LogContext, dir, prefix string) {
	// only perform cleanup once and only if cache dir is set
	if !atomic.CompareAndSwapInt32(&alreadyCleanedupOutdated, 0, 1) || dir == "" {
		return
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		logger.Warnf("reading cache dir %s failed with %s", dir, err)
		return
	}
	outdated := time.Now().Add(-24 * time.Hour)

outer:
	for _, file := range files {
		if file.IsDir() && strings.HasPrefix(file.Name(), prefix) && file.ModTime().Before(outdated) {
			subDir := filepath.Join(dir, file.Name())
			subdirFiles, err := ioutil.ReadDir(subDir)
			if err != nil {
				continue
			}
			for _, subfile := range subdirFiles {
				if subfile.IsDir() || !subfile.ModTime().Before(outdated) {
					continue outer
				}
				err = os.Remove(filepath.Join(subDir, subfile.Name()))
			}
			err = os.Remove(subDir)
			if err != nil {
				logger.Warnf("deleting outdated cache dir %s failed with %s", subDir, err)
			} else {
				logger.Infof("deleted outdated cache dir %s", subDir)
			}
		}
	}
}

func (c *defaultZoneCache) restoreFromDisk() error {
	dir := c.config.persistDir
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("restoring zone cache from %s failed with %s", dir, err)
	}

	zones := DNSHostedZones{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if strings.HasSuffix(file.Name(), TempSuffix) ||
			file.ModTime().Add(24*time.Hour).Before(time.Now()) {
			_ = os.Remove(filepath.Join(dir, file.Name()))
			continue
		}
		zone, err := c.state.ReadZone(file.Name())
		if err != nil {
			c.logger.Info(err)
		}
		if zone == nil {
			_ = os.Remove(filepath.Join(dir, file.Name()))
			continue
		}
		zones = append(zones, zone)
		c.zonesNext = time.Time{} // enforces sync of zones
		c.zones = zones
	}
	return nil
}

type zoneState struct {
	lock        sync.Mutex
	persistDir  string
	ttl         time.Duration
	inMemory    *InMemory
	next        map[string]time.Time
	handlerData HandlerData
}

func (s *zoneState) GetZoneState(zone DNSHostedZone, cache *defaultZoneCache) (DNSZoneState, bool, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	next, ok := s.next[zone.Id()]
	if !ok || time.Now().After(next) {
		state, err := cache.stateUpdater(zone, cache)
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

func (s *zoneState) GetHandlerData() HandlerData {
	return s.handlerData
}

func (s *zoneState) DeleteZoneState(zone DNSHostedZone) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.deleteZoneState(zone)
}

func (s *zoneState) deleteZoneState(zone DNSHostedZone) {
	delete(s.next, zone.Id())
	s.inMemory.DeleteZone(zone)
	if s.handlerData != nil {
		s.handlerData.DeleteZone(zone.Id())
	}
	go s.deletePersistedZone(zone.Id())
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

func (s *zoneState) ReadZone(filename string) (DNSHostedZone, error) {
	jsonFile, err := os.Open(filepath.Join(s.persistDir, filename))
	if err != nil {
		return nil, fmt.Errorf("opening zone cache file %s failed with %s", filename, err)
	}
	bytes, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return nil, fmt.Errorf("reading zone cache file %s failed with %s", filename, err)
	}

	persistentState := &PersistentZoneState{}
	err = json.Unmarshal(bytes, persistentState)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling zone cache from file %s failed with %s", filename, err)
	}

	if persistentState.Version != "1" {
		return nil, fmt.Errorf("invalid version %s for zone cache from file %s", persistentState.Version, filename)
	}
	if time.Now().After(persistentState.Valid) {
		return nil, nil
	}

	return s.RestoreZone(persistentState), nil
}

func (s *zoneState) buildFilename(zoneid string) string {
	return filepath.Join(s.persistDir, strings.ReplaceAll(zoneid, "/", "_"))
}

func (s *zoneState) WriteZone(zoneid string) error {
	state := s.BuildPersistentZoneState(zoneid)
	if state == nil {
		s.deletePersistedZone(zoneid)
		return nil
	}

	data, err := json.MarshalIndent(state, "", " ")
	if err != nil {
		return fmt.Errorf("marshalling zone cache for %s failed with %s", zoneid, err)
	}

	filename := s.buildFilename(zoneid)
	tmpName := filename + TempSuffix
	err = ioutil.WriteFile(tmpName, data, 0644)
	if err != nil {
		return fmt.Errorf("writing zone cache for %s failed with %s", zoneid, err)
	}

	err = os.Rename(tmpName, filename)
	if err != nil {
		return fmt.Errorf("renaming zone cache for %s failed with %s", zoneid, err)
	}

	return nil
}

func (s *zoneState) deletePersistedZone(zoneid string) error {
	filename := s.buildFilename(zoneid)
	return os.Remove(filename)
}

func (s *zoneState) BuildPersistentZoneState(zoneid string) *PersistentZoneState {
	s.lock.Lock()
	defer s.lock.Unlock()

	zone := s.inMemory.FindHostedZone(zoneid)
	if zone == nil {
		return nil
	}
	persistentState := &PersistentZoneState{
		Version: "1",
		Zone:    *NewPersistentZone(zone),
		Valid:   s.next[zoneid],
	}

	state, err := s.inMemory.CloneZoneState(zone)
	if err == nil {
		persistentState.DNSSets = state.GetDNSSets()
	}

	hd := s.GetHandlerData()
	if hd != nil {
		value, _ := hd.Marshal(zone.Id())
		persistentState.HandlerData = value
	}

	return persistentState
}

func (s *zoneState) RestoreZone(persistentState *PersistentZoneState) DNSHostedZone {
	zone := persistentState.Zone.ToDNSHostedZone()
	zoneState := NewDNSZoneState(persistentState.DNSSets)
	s.inMemory.SetZone(zone, zoneState)
	s.next[zone.Id()] = persistentState.Valid
	if persistentState.HandlerData != nil && s.GetHandlerData() != nil {
		_ = s.GetHandlerData().Unmarshal(zone.Id(), persistentState.HandlerData)
	}
	return zone
}
