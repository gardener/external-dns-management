// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"errors"
	"time"

	"github.com/jellydator/ttlcache/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("DNSCache", func() {
	var (
		ctx          context.Context
		dnsCache     *DNSCache
		mockQuery    *mockQueryDNS
		sampleResult QueryDNSResult
	)

	BeforeEach(func() {
		ctx = context.Background()
		sampleResult = QueryDNSResult{
			Records: []dns.Record{{Value: "1.2.3.4"}},
			TTL:     1,
		}

		mockQuery = &mockQueryDNS{
			records: map[string]map[dns.RecordType][]dns.Record{
				"example.com.": map[dns.RecordType][]dns.Record{
					dns.TypeA: []dns.Record{{Value: "1.2.3.4"}},
				},
			},
			ttl: 1,
		}

		dnsCache = NewDNSCache(mockQuery, 5*time.Minute)
	})

	Describe("Get", func() {
		It("should query DNS and cache the result if not present", func() {
			for i := 0; i < 7; i++ {
				result := dnsCache.Get(ctx, "example.com", dns.TypeA)
				Expect(result.Err).NotTo(HaveOccurred())
				Expect(result.TTL).To(Equal(uint32(1)))
				Expect(result.Records).To(ConsistOf(dns.Record{Value: "1.2.3.4"}))
				if i < 3 {
					Expect(mockQuery.queryCount).To(Equal(1))
				} else if i > 4 {
					Expect(mockQuery.queryCount).To(Equal(2))
				}
				time.Sleep(250 * time.Millisecond)
			}
		})
	})

	Describe("Len and Clear", func() {
		It("should remove all entries from the cache", func() {
			dnsCache.cache.Set(cacheKey{fqdn: "example.com.", rstype: dns.TypeA}, sampleResult, ttlcache.DefaultTTL)

			Expect(dnsCache.cache.Len()).To(Equal(1))
			dnsCache.Clear()
			Expect(dnsCache.cache.Len()).To(BeZero())
		})
	})
})

type mockQueryDNS struct {
	records    map[string]map[dns.RecordType][]dns.Record
	ttl        uint32
	queryCount int
}

func (m *mockQueryDNS) Query(_ context.Context, fqdn string, rtype dns.RecordType) QueryDNSResult {
	rsmap := m.records[fqdn]
	if rsmap == nil {
		return QueryDNSResult{TTL: 10, Err: errors.New("domain name not found")}
	}
	rs := rsmap[rtype]
	if rs == nil {
		return QueryDNSResult{TTL: 10, Err: errors.New("record type not found")}
	}

	m.queryCount++
	return QueryDNSResult{Records: rs, TTL: m.ttl}
}
