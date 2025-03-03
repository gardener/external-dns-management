// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/errors"
	"github.com/gardener/external-dns-management/pkg/server/metrics"
)

type StateTTLGetter func(zoneid dns.ZoneID) time.Duration

type ZoneCacheFactory struct {
	context               context.Context
	logger                logger.LogContext
	zonesTTL              time.Duration
	zoneStates            *zoneStates
	disableZoneStateCache bool
}

func (c ZoneCacheFactory) CreateZoneCache(cacheType ZoneCacheType, metrics Metrics, zonesUpdater ZoneCacheZoneUpdater, stateUpdater ZoneCacheStateUpdater) (ZoneCache, error) {
	cache := onlyZonesCache{zonesTTL: c.zonesTTL, logger: c.logger, metrics: metrics, zonesUpdater: zonesUpdater, stateUpdater: stateUpdater}
	switch cacheType {
	case CacheZonesOnly:
		return &cache, nil
	case CacheZoneState:
		if c.disableZoneStateCache {
			return &cache, nil
		}
		return &defaultZoneCache{onlyZonesCache: onlyZonesCache{zonesTTL: c.zonesTTL, logger: c.logger, metrics: metrics, zonesUpdater: zonesUpdater, stateUpdater: stateUpdater}, zoneStates: c.zoneStates}, nil
	default:
		return nil, fmt.Errorf("unknown zone cache type: %v", cacheType)
	}
}

// ZoneCacheType is the zone cache type.
type ZoneCacheType int

const (
	// CacheZonesOnly only caches the zones of the account, but not the zone state itself.
	CacheZonesOnly ZoneCacheType = iota
	// CacheZoneState caches both zones of the account and the zone states as needed.
	CacheZoneState
)

func NewTestZoneCacheFactory(zonesTTL, stateTTL time.Duration) *ZoneCacheFactory {
	return &ZoneCacheFactory{
		zonesTTL:   zonesTTL,
		zoneStates: newZoneStates(func(_ dns.ZoneID) time.Duration { return stateTTL }),
	}
}

type ZoneCacheZoneUpdater func(cache ZoneCache) (DNSHostedZones, error)

type ZoneCacheStateUpdater func(zone DNSHostedZone, cache ZoneCache) (DNSZoneState, error)

type ZoneCache interface {
	GetZones() (DNSHostedZones, error)
	GetZoneState(zone DNSHostedZone) (DNSZoneState, error)
	ApplyRequests(logctx logger.LogContext, err error, zone DNSHostedZone, reqs []*ChangeRequest)
	Release()
}

type onlyZonesCache struct {
	lock         sync.Mutex
	logger       logger.LogContext
	metrics      Metrics
	zonesTTL     time.Duration
	zones        DNSHostedZones
	zonesErr     error
	zonesNext    time.Time
	zonesUpdater ZoneCacheZoneUpdater
	stateUpdater ZoneCacheStateUpdater

	backoffOnError time.Duration
}

var _ ZoneCache = &onlyZonesCache{}

func (c *onlyZonesCache) GetZones() (DNSHostedZones, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if time.Now().After(c.zonesNext) {
		c.zones, c.zonesErr = c.zonesUpdater(c)
		updateTime := time.Now()
		if c.zonesErr != nil {
			// if getzones fails, don't wait zonesTTL, but use an exponential backoff
			// to recover fast from temporary failures like throttling, network problems...
			backoff := c.nextBackoff()
			c.zonesNext = updateTime.Add(backoff)
		} else {
			c.clearBackoff()
			c.zonesNext = updateTime.Add(c.zonesTTL)
		}
	} else {
		c.metrics.AddGenericRequests(M_CACHED_GETZONES, 1)
	}
	return c.zones, c.zonesErr
}

func (c *onlyZonesCache) nextBackoff() time.Duration {
	next := c.backoffOnError*5/4 + 2*time.Second
	maxBackoff := c.zonesTTL / 4
	if next > maxBackoff {
		next = maxBackoff
	}
	c.backoffOnError = next
	return next
}

func (c *onlyZonesCache) clearBackoff() {
	c.backoffOnError = 0
}

func (c *onlyZonesCache) GetZoneState(zone DNSHostedZone) (DNSZoneState, error) {
	state, err := c.stateUpdater(zone, c)
	return state, err
}

func (c *onlyZonesCache) ApplyRequests(_ logger.LogContext, _ error, _ DNSHostedZone, _ []*ChangeRequest) {
}

func (c *onlyZonesCache) Release() {
}

type defaultZoneCache struct {
	onlyZonesCache
	zoneStates *zoneStates
}

var _ ZoneCache = &defaultZoneCache{}

func (c *defaultZoneCache) GetZones() (DNSHostedZones, error) {
	old := c.zonesNext
	zones, err := c.onlyZonesCache.GetZones()
	if c.zonesNext != old {
		c.zoneStates.UpdateUsedZones(c, toSortedZoneIDs(c.zones))
	}
	return zones, err
}

func (c *defaultZoneCache) GetZoneState(zone DNSHostedZone) (DNSZoneState, error) {
	state, cached, err := c.zoneStates.GetZoneState(zone, c)
	if cached {
		c.metrics.AddZoneRequests(zone.Id().ID, M_CACHED_GETZONESTATE, 1)
	}
	return state, err
}

func (c *defaultZoneCache) cleanZoneState(zoneID dns.ZoneID) {
	c.zoneStates.CleanZoneState(zoneID)
}

func (c *defaultZoneCache) ApplyRequests(logctx logger.LogContext, err error, zone DNSHostedZone, reqs []*ChangeRequest) {
	if err == nil {
		c.zoneStates.ExecuteRequests(zone.Id(), reqs)
	} else {
		if !errors.IsThrottlingError(err) {
			logctx.Infof("zone cache discarded because of error during ExecuteRequests")
			c.cleanZoneState(zone.Id())
			metrics.AddZoneCacheDiscarding(zone.Id())
		} else {
			logctx.Infof("zone cache untouched (only throttling during ExecuteRequests)")
		}
	}
}

func (c *defaultZoneCache) Release() {
	c.zoneStates.UpdateUsedZones(c, nil)
}

type zoneStateProxy struct {
	lock            sync.Mutex
	lastUpdateStart time.Time
	lastUpdateEnd   time.Time
}

type zoneStates struct {
	lock           sync.Mutex
	stateTTLGetter StateTTLGetter
	inMemory       *InMemory
	proxies        map[dns.ZoneID]*zoneStateProxy
	usedZones      map[ZoneCache][]dns.ZoneID
}

func newZoneStates(stateTTLGetter StateTTLGetter) *zoneStates {
	return &zoneStates{
		inMemory:       NewInMemory(),
		stateTTLGetter: stateTTLGetter,
		proxies:        map[dns.ZoneID]*zoneStateProxy{},
		usedZones:      map[ZoneCache][]dns.ZoneID{},
	}
}

func (s *zoneStates) getProxy(zoneID dns.ZoneID) *zoneStateProxy {
	s.lock.Lock()
	defer s.lock.Unlock()
	proxy := s.proxies[zoneID]
	if proxy == nil {
		proxy = &zoneStateProxy{}
		s.proxies[zoneID] = proxy
	}
	return proxy
}

func (s *zoneStates) GetZoneState(zone DNSHostedZone, cache *defaultZoneCache) (DNSZoneState, bool, error) {
	proxy := s.getProxy(zone.Id())
	proxy.lock.Lock()
	defer proxy.lock.Unlock()

	start := time.Now()
	ttl := s.stateTTLGetter(zone.Id())
	if start.After(proxy.lastUpdateEnd.Add(ttl)) {
		state, err := cache.stateUpdater(zone, cache)
		if err == nil {
			proxy.lastUpdateStart = start
			proxy.lastUpdateEnd = time.Now()
			s.inMemory.SetZone(zone, state)
		} else {
			s.cleanZoneState(zone.Id(), proxy)
		}
		return state, false, err
	}

	state, err := s.inMemory.CloneZoneState(zone)
	if err != nil {
		return nil, true, err
	}
	return state, true, nil
}

func (s *zoneStates) ExecuteRequests(zoneID dns.ZoneID, reqs []*ChangeRequest) {
	proxy := s.getProxy(zoneID)
	proxy.lock.Lock()
	defer proxy.lock.Unlock()

	var err error
	nullMetrics := &NullMetrics{}
	for _, req := range reqs {
		if req.Applied {
			err = s.inMemory.Apply(zoneID, req, nullMetrics)
			if err != nil {
				break
			}
		}
	}

	if err != nil {
		s.cleanZoneState(zoneID, proxy)
	}
}

func (s *zoneStates) CleanZoneState(zoneID dns.ZoneID) {
	control := s.getProxy(zoneID)
	control.lock.Lock()
	defer control.lock.Unlock()

	s.cleanZoneState(zoneID, control)
}

func (s *zoneStates) cleanZoneState(zoneID dns.ZoneID, proxy *zoneStateProxy) {
	s.inMemory.DeleteZone(zoneID)
	if proxy != nil {
		var zero time.Time
		proxy.lastUpdateStart = zero
		proxy.lastUpdateEnd = zero
	}
}

func (s *zoneStates) UpdateUsedZones(cache ZoneCache, zoneids []dns.ZoneID) {
	s.lock.Lock()
	defer s.lock.Unlock()

	oldids := s.usedZones[cache]
	if len(zoneids) == 0 {
		if len(oldids) == 0 {
			return
		}
		delete(s.usedZones, cache)
	} else {
		if reflect.DeepEqual(oldids, zoneids) {
			return
		}
		s.usedZones[cache] = zoneids
	}

	allUsed := map[dns.ZoneID]struct{}{}
	for _, zoneids := range s.usedZones {
		for _, id := range zoneids {
			allUsed[id] = struct{}{}
		}
	}

	for _, zone := range s.inMemory.GetZones() {
		if _, ok := allUsed[zone.Id()]; !ok {
			s.cleanZoneState(zone.Id(), nil)
		}
	}
	for id := range s.proxies {
		if _, ok := allUsed[id]; !ok {
			delete(s.proxies, id)
		}
	}
}

func toSortedZoneIDs(zones DNSHostedZones) []dns.ZoneID {
	if len(zones) == 0 {
		return nil
	}
	zoneids := make([]dns.ZoneID, len(zones))
	for i, zone := range zones {
		zoneids[i] = zone.Id()
	}
	sort.Slice(zoneids, func(i, j int) bool {
		cmp1 := strings.Compare(zoneids[i].ProviderType, zoneids[j].ProviderType)
		cmp2 := strings.Compare(zoneids[i].ID, zoneids[j].ID)
		return cmp1 < 0 || cmp1 == 0 && cmp2 < 0
	})
	return zoneids
}
