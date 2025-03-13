// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"time"

	"github.com/jellydator/ttlcache/v3"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// DNSCache is a TTL cache for DNS records.
type DNSCache struct {
	dnsQuery QueryDNS
	cache    *ttlcache.Cache[cacheKey, QueryDNSResult]
}

type cacheKey struct {
	fqdn   string
	rstype dns.RecordType
}

// NewDNSCache creates a new DNSCache.
func NewDNSCache(dnsQuery QueryDNS, defaultTTL time.Duration) *DNSCache {
	return &DNSCache{
		dnsQuery: dnsQuery,
		cache: ttlcache.New[cacheKey, QueryDNSResult](
			ttlcache.WithTTL[cacheKey, QueryDNSResult](defaultTTL),
			ttlcache.WithDisableTouchOnHit[cacheKey, QueryDNSResult](),
		),
	}
}

// Get returns the DNS records for the given DNS name and record type.
func (c *DNSCache) Get(ctx context.Context, dnsName string, rstype dns.RecordType) QueryDNSResult {
	key := cacheKey{ToFQDN(dnsName), rstype}
	item := c.cache.Get(key)
	if item != nil {
		return item.Value()
	}
	result := c.dnsQuery.Query(ctx, key.fqdn, rstype)
	c.cache.Set(key, result, time.Duration(result.TTL)*time.Second)
	return result
}

// Clear removes all entries from the cache.
func (c *DNSCache) Clear() {
	c.cache.DeleteAll()
}

// Len returns the number of entries in the cache.
func (c *DNSCache) Len() int {
	return c.cache.Len()
}
