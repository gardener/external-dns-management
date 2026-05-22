// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"net"

	miekgdns "github.com/miekg/dns"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
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
		Expect(result.RecordSet.Records).To(ConsistOf(&dns.Record{Value: "hera.ns.cloudflare.com."}, &dns.Record{Value: "elliott.ns.cloudflare.com."}))
	})

	It("should return CNAME records", func() {
		result := queryDNS.Query(ctx, dns.DNSSetName{DNSName: "www.sap.com"}, dns.TypeCNAME)
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

var _ = Describe("QueryDNS with mocked DNS responses", func() {
	var (
		ctx     context.Context
		setName = dns.DNSSetName{DNSName: "example.com"}
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// makeQueryDNS builds a standardQueryDNS whose dnsQueryFn returns the supplied answer set.
	makeQueryDNS := func(answer []miekgdns.RR, rcode int) QueryDNS {
		q := &standardQueryDNS{
			nameservers: mockNameserversProvider{nameservers: []string{"unused"}},
		}
		q.dnsQueryFn = func(_ context.Context, _ string, _ uint16) (*miekgdns.Msg, error) {
			msg := new(miekgdns.Msg)
			msg.Rcode = rcode
			msg.Answer = answer
			return msg, nil
		}
		return q
	}

	cnameRR := func(name, target string, ttl uint32) *miekgdns.CNAME {
		return &miekgdns.CNAME{
			Hdr:    miekgdns.RR_Header{Name: name, Rrtype: miekgdns.TypeCNAME, Class: miekgdns.ClassINET, Ttl: ttl},
			Target: target,
		}
	}
	aRR := func(name, ip string, ttl uint32) *miekgdns.A {
		return &miekgdns.A{
			Hdr: miekgdns.RR_Header{Name: name, Rrtype: miekgdns.TypeA, Class: miekgdns.ClassINET, Ttl: ttl},
			A:   net.ParseIP(ip).To4(),
		}
	}
	aaaaRR := func(name, ip string, ttl uint32) *miekgdns.AAAA {
		return &miekgdns.AAAA{
			Hdr:  miekgdns.RR_Header{Name: name, Rrtype: miekgdns.TypeAAAA, Class: miekgdns.ClassINET, Ttl: ttl},
			AAAA: net.ParseIP(ip),
		}
	}
	mxRR := func(name string, ttl uint32) *miekgdns.MX {
		return &miekgdns.MX{
			Hdr:        miekgdns.RR_Header{Name: name, Rrtype: miekgdns.TypeMX, Class: miekgdns.ClassINET, Ttl: ttl},
			Preference: 10,
			Mx:         "mail.example.com.",
		}
	}

	DescribeTable("record-type / answer combinations",
		func(rstype dns.RecordType, answer []miekgdns.RR, rcode int, errMatcher types.GomegaMatcher, recordsMatcher types.GomegaMatcher) {
			q := makeQueryDNS(answer, rcode)
			result := q.Query(ctx, setName, rstype)
			Expect(result.Err).To(errMatcher)
			if result.Err == nil && result.RecordSet != nil {
				Expect(result.RecordSet.Records).To(recordsMatcher)
			} else if result.Err == nil {
				Expect(result.RecordSet).To(BeNil())
			}
		},
		Entry("A query with only CNAME RR returns no records and no error",
			dns.TypeA,
			[]miekgdns.RR{cnameRR("example.com.", "alias.example.net.", 60)},
			miekgdns.RcodeSuccess,
			Not(HaveOccurred()),
			BeEmpty(),
		),
		Entry("AAAA query with only CNAME RR returns no records and no error",
			dns.TypeAAAA,
			[]miekgdns.RR{cnameRR("example.com.", "alias.example.net.", 60)},
			miekgdns.RcodeSuccess,
			Not(HaveOccurred()),
			BeEmpty(),
		),
		Entry("A query with mixed CNAME + A RRs discards A records",
			dns.TypeA,
			[]miekgdns.RR{
				cnameRR("example.com.", "alias.example.net.", 60),
				aRR("alias.example.net.", "1.2.3.4", 60),
			},
			miekgdns.RcodeSuccess,
			Not(HaveOccurred()),
			BeEmpty(),
		),
		Entry("AAAA query with mixed CNAME + AAAA RRs discards AAAA records",
			dns.TypeAAAA,
			[]miekgdns.RR{
				cnameRR("example.com.", "alias.example.net.", 60),
				aaaaRR("alias.example.net.", "2001:db8::1", 60),
			},
			miekgdns.RcodeSuccess,
			Not(HaveOccurred()),
			BeEmpty(),
		),
		Entry("A query with truly unexpected RR type errors",
			dns.TypeA,
			[]miekgdns.RR{mxRR("example.com.", 60)},
			miekgdns.RcodeSuccess,
			MatchError(ContainSubstring("unexpected record type")),
			nil,
		),
		Entry("AAAA query with truly unexpected RR type errors",
			dns.TypeAAAA,
			[]miekgdns.RR{mxRR("example.com.", 60)},
			miekgdns.RcodeSuccess,
			MatchError(ContainSubstring("unexpected record type")),
			nil,
		),
		Entry("CNAME query receiving A record errors (inverse mismatch)",
			dns.TypeCNAME,
			[]miekgdns.RR{aRR("example.com.", "1.2.3.4", 60)},
			miekgdns.RcodeSuccess,
			MatchError(ContainSubstring("unexpected record type")),
			nil,
		),
		Entry("NXDOMAIN returns nil record set without error",
			dns.TypeA,
			nil,
			miekgdns.RcodeNameError,
			Not(HaveOccurred()),
			nil,
		),
		Entry("A query with only A RRs returns those records",
			dns.TypeA,
			[]miekgdns.RR{
				aRR("example.com.", "1.2.3.4", 60),
				aRR("example.com.", "5.6.7.8", 120),
			},
			miekgdns.RcodeSuccess,
			Not(HaveOccurred()),
			ConsistOf(&dns.Record{Value: "1.2.3.4"}, &dns.Record{Value: "5.6.7.8"}),
		),
	)
})
