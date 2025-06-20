// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils_test

import (
	"context"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	. "github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var _ = Describe("ToFQDN", func() {
	It("should add a trailing dot if not present", func() {
		Expect(ToFQDN("example.com")).To(Equal("example.com."))
	})

	It("should return the same domain if trailing dot is present", func() {
		Expect(ToFQDN("example.com.")).To(Equal("example.com."))
	})
})

type mockNameserversProvider struct {
	nameservers []string
}

func (m mockNameserversProvider) Nameservers(_ context.Context) ([]string, error) {
	return m.nameservers, nil
}

var _ = Describe("QueryDNS", func() {
	var (
		mockNameservers NameserversProvider
		queryDNS        QueryDNS
		ctx             context.Context
		setName         = dns.DNSSetName{DNSName: "example.com"}
	)

	BeforeEach(func() {
		mockNameservers = mockNameserversProvider{nameservers: []string{"8.8.8.8:53"}}
		queryDNS = NewStandardQueryDNS(mockNameservers)
		ctx = context.Background()
	})

	It("should return A records", func() {
		result := queryDNS.Query(ctx, setName, dns.TypeA)
		Expect(result.Err).NotTo(HaveOccurred())
		Expect(result.RecordSet).NotTo(BeNil())
		Expect(result.RecordSet.TTL).NotTo(BeZero())
		Expect(result.RecordSet.Records).NotTo(BeEmpty())
		for _, record := range result.RecordSet.Records {
			ip := net.ParseIP(record.Value)
			Expect(ip).NotTo(BeNil())
			Expect(ip.To4()).NotTo(BeNil())
		}
	})

	It("should return AAAA records", func() {
		result := queryDNS.Query(ctx, setName, dns.TypeAAAA)
		Expect(result.Err).NotTo(HaveOccurred())
		Expect(result.RecordSet).NotTo(BeNil())
		Expect(result.RecordSet.TTL).NotTo(BeZero())
		Expect(result.RecordSet.Records).NotTo(BeEmpty())
		for _, record := range result.RecordSet.Records {
			ip := net.ParseIP(record.Value)
			Expect(ip).NotTo(BeNil())
			Expect(ip.To16()).NotTo(BeNil())
		}
	})

	It("should return TXT records", func() {
		result := queryDNS.Query(ctx, setName, dns.TypeTXT)
		Expect(result.RecordSet).NotTo(BeNil())
		Expect(result.RecordSet.TTL).NotTo(BeZero())
		Expect(result.Err).NotTo(HaveOccurred())
		Expect(result.RecordSet.Records).NotTo(BeEmpty())
	})

	It("should return NS records", func() {
		result := queryDNS.Query(ctx, setName, dns.TypeNS)
		Expect(result.Err).NotTo(HaveOccurred())
		Expect(result.RecordSet).NotTo(BeNil())
		Expect(result.RecordSet.TTL).NotTo(BeZero())
		Expect(result.RecordSet.Records).To(ConsistOf(&dns.Record{Value: "a.iana-servers.net."}, &dns.Record{Value: "b.iana-servers.net."}))
	})

	It("should return CNAME records", func() {
		result := queryDNS.Query(ctx, dns.DNSSetName{DNSName: "www.example.com"}, dns.TypeCNAME)
		Expect(result.Err).NotTo(HaveOccurred())
		Expect(result.RecordSet).NotTo(BeNil())
		Expect(result.RecordSet.TTL).NotTo(BeZero())
		Expect(result.RecordSet.Records).NotTo(BeEmpty())
	})

	It("should return an error for unsupported record type", func() {
		result := queryDNS.Query(ctx, setName, dns.TypeAWS_ALIAS_A)
		Expect(result.Err).To(HaveOccurred())
		Expect(result.Err.Error()).To(ContainSubstring("unsupported record type"))
	})

	It("should return an error if setIdentifier is set", func() {
		result := queryDNS.Query(ctx, dns.DNSSetName{DNSName: "example.com", SetIdentifier: "set1"}, dns.TypeA)
		Expect(result.Err).To(HaveOccurred())
		Expect(result.Err.Error()).To(ContainSubstring("set identifier is not supported for DNS queries"))
	})
})
