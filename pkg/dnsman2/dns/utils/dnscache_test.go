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
			RecordSet: &dns.RecordSet{
				Type:    dns.TypeA,
				Records: []*dns.Record{{Value: "1.2.3.4"}},
				TTL:     1,
			},
		}

		mockQuery = &mockQueryDNS{
			records: map[string]map[dns.RecordType][]*dns.Record{
				"example.com.": {
					dns.TypeA: []*dns.Record{{Value: "1.2.3.4"}},
				},
			},
			ttl: 1,
		}

		dnsCache = NewDNSCache(mockQuery, 5*time.Minute)
	})

	Describe("Get", func() {
		It("should query DNS and cache the result if not present", func() {
			for i := range 7 {
				rs, err := dnsCache.Get(ctx, dns.DNSSetName{DNSName: "example.com"}, dns.TypeA)
				Expect(err).NotTo(HaveOccurred())
				Expect(rs).NotTo(BeNil())
				Expect(rs.TTL).To(Equal(int64(1)))
				Expect(rs.Records).To(ConsistOf(&dns.Record{Value: "1.2.3.4"}))
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
			dnsCache.cache.Set(RecordSetKey{Name: dns.DNSSetName{DNSName: "example.com"}, RecordType: dns.TypeA}, sampleResult, ttlcache.DefaultTTL)

			Expect(dnsCache.cache.Len()).To(Equal(1))
			dnsCache.Clear()
			Expect(dnsCache.cache.Len()).To(BeZero())
		})
	})
})

type mockQueryDNS struct {
	records    map[string]map[dns.RecordType][]*dns.Record
	ttl        uint32
	queryCount int
}

func (m *mockQueryDNS) Query(_ context.Context, setName dns.DNSSetName, rtype dns.RecordType) QueryDNSResult {
	rsmap := m.records[setName.EnsureTrailingDot().DNSName]
	if rsmap == nil {
		return QueryDNSResult{Err: errors.New("domain name not found")}
	}
	records := rsmap[rtype]
	if records == nil {
		return QueryDNSResult{Err: errors.New("record type not found")}
	}

	m.queryCount++
	return QueryDNSResult{RecordSet: dns.NewRecordSet(rtype, int64(m.ttl), records)}
}
