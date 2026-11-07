// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/jellydator/ttlcache/v3"
)

// DNSCache is a TTL cache for DNS records.
type DNSCache struct {
	dnsQuery QueryDNS
	cache    *ttlcache.Cache[QueryDNSRequest, QueryDNSResult]
	ttl      time.Duration
}

// NewDNSCache creates a new DNSCache.
func NewDNSCache(dnsQuery QueryDNS, ttl time.Duration) *DNSCache {
	return &DNSCache{
		dnsQuery: dnsQuery,
		cache: ttlcache.New[QueryDNSRequest, QueryDNSResult](
			ttlcache.WithTTL[QueryDNSRequest, QueryDNSResult](ttl),
			ttlcache.WithDisableTouchOnHit[QueryDNSRequest, QueryDNSResult](),
		),
		ttl: ttl,
	}
}

// Get returns the DNS records for the given DNS name and record type.
func (c *DNSCache) Get(ctx context.Context, log logr.Logger, request QueryDNSRequest) QueryDNSResult {
	item := c.cache.Get(request)
	if item != nil {
		log.V(2).Info("found in cache", "key", request)
		return item.Value()
	}
	result := c.dnsQuery.Query(ctx, request)
	if result.Err != nil || result.RecordSet == nil || len(result.RecordSet.Records) == 0 {
		log.V(2).Info("caching negative result", "key", request, "ttl", c.ttl)
		c.cache.Set(request, result, c.ttl)
	} else {
		// Cap the cache TTL at the configured c.ttl: the record's TTL is attacker-controllable via a
		// spoofed answer (plain UDP, no DNSSEC), and an uncapped value would let one forged response
		// suppress corrective writes for up to ~136 years. Capping bounds any poisoning to one reconcile window.
		ttl := time.Duration(result.RecordSet.TTL) * time.Second
		if ttl > c.ttl {
			ttl = c.ttl
		}
		log.V(2).Info("caching query result", "key", request, "ttl", ttl)
		c.cache.Set(request, result, ttl)
	}
	return result
}

// Clear removes all entries from the cache.
func (c *DNSCache) Clear() {
	c.cache.DeleteAll()
}

// ClearKeys removes specific keys from the cache.
func (c *DNSCache) ClearKeys(log logr.Logger, keys ...QueryDNSRequest) {
	c.cache.DeleteExpired()
	for _, key := range keys {
		c.cache.Delete(key)
		log.V(2).Info("clearing from cache", "key", key)
	}
}

// Len returns the number of entries in the cache.
func (c *DNSCache) Len() int {
	return c.cache.Len()
}
