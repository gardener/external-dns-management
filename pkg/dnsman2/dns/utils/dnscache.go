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
	cache    *ttlcache.Cache[RecordSetKey, QueryDNSResult]
}

// RecordSetKey represents a key for a DNS record set, including name and record type.
type RecordSetKey struct {
	Name       dns.DNSSetName
	RecordType dns.RecordType
}

const negativeTTL = 15 * time.Second

// NewDNSCache creates a new DNSCache.
func NewDNSCache(dnsQuery QueryDNS, defaultTTL time.Duration) *DNSCache {
	return &DNSCache{
		dnsQuery: dnsQuery,
		cache: ttlcache.New[RecordSetKey, QueryDNSResult](
			ttlcache.WithTTL[RecordSetKey, QueryDNSResult](defaultTTL),
			ttlcache.WithDisableTouchOnHit[RecordSetKey, QueryDNSResult](),
		),
	}
}

// Get returns the DNS records for the given DNS name and record type.
func (c *DNSCache) Get(ctx context.Context, setName dns.DNSSetName, rstype dns.RecordType) (*dns.RecordSet, error) {
	key := RecordSetKey{Name: setName, RecordType: rstype}
	item := c.cache.Get(key)
	if item != nil {
		return item.Value().RecordSet, item.Value().Err
	}
	result := c.dnsQuery.Query(ctx, setName, rstype)
	if result.Err != nil || result.RecordSet == nil || len(result.RecordSet.Records) == 0 {
		c.cache.Set(key, result, negativeTTL)
	} else {
		c.cache.Set(key, result, time.Duration(result.RecordSet.TTL)*time.Second)
	}
	return result.RecordSet, result.Err
}

// Clear removes all entries from the cache.
func (c *DNSCache) Clear() {
	c.cache.DeleteAll()
}

// ClearKeys removes specific keys from the cache.
func (c *DNSCache) ClearKeys(keys ...RecordSetKey) {
	c.cache.DeleteExpired()
	for _, key := range keys {
		c.cache.Delete(key)
	}
}

// Len returns the number of entries in the cache.
func (c *DNSCache) Len() int {
	return c.cache.Len()
}
