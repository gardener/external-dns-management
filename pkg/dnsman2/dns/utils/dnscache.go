// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/jellydator/ttlcache/v3"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// DNSCache is a TTL cache for DNS records.
type DNSCache struct {
	dnsQuery QueryDNS
	cache    *ttlcache.Cache[RecordSetKey, QueryDNSResult]
	ttl      time.Duration
}

// RecordSetKey represents a key for a DNS record set, including name and record type.
type RecordSetKey struct {
	Name       dns.DNSSetName
	RecordType dns.RecordType
}

func (k RecordSetKey) String() string {
	return k.Name.String() + "/" + string(k.RecordType)
}

// NewDNSCache creates a new DNSCache.
func NewDNSCache(dnsQuery QueryDNS, ttl time.Duration) *DNSCache {
	return &DNSCache{
		dnsQuery: dnsQuery,
		cache: ttlcache.New[RecordSetKey, QueryDNSResult](
			ttlcache.WithTTL[RecordSetKey, QueryDNSResult](ttl),
			ttlcache.WithDisableTouchOnHit[RecordSetKey, QueryDNSResult](),
		),
		ttl: ttl,
	}
}

// Get returns the DNS records for the given DNS name and record type.
func (c *DNSCache) Get(ctx context.Context, log logr.Logger, setName dns.DNSSetName, rstype dns.RecordType) (*dns.RecordSet, error) {
	key := RecordSetKey{Name: setName, RecordType: rstype}
	item := c.cache.Get(key)
	if item != nil {
		log.V(2).Info("found in cache", "key", key)
		return item.Value().RecordSet, item.Value().Err
	}
	result := c.dnsQuery.Query(ctx, setName, rstype)
	if result.Err != nil || result.RecordSet == nil || len(result.RecordSet.Records) == 0 {
		log.V(2).Info("caching negative result", "key", key, "ttl", c.ttl)
		c.cache.Set(key, result, c.ttl)
	} else {
		log.V(2).Info("caching query result", "key", key, "ttl", c.ttl)
		c.cache.Set(key, result, time.Duration(result.RecordSet.TTL)*time.Second)
	}
	return result.RecordSet, result.Err
}

// Clear removes all entries from the cache.
func (c *DNSCache) Clear() {
	c.cache.DeleteAll()
}

// ClearKeys removes specific keys from the cache.
func (c *DNSCache) ClearKeys(log logr.Logger, keys ...RecordSetKey) {
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
