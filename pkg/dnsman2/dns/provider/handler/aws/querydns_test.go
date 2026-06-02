// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("queryDNS wildcard / TXT fixes", func() {
	var (
		ctx  context.Context
		fake *fakeRoute53
	)

	BeforeEach(func() {
		ctx = logr.NewContext(context.Background(), logr.Discard())
		fake = &fakeRoute53{}
	})

	Describe("unescape", func() {
		It("converts the Route53 wildcard escape prefix back to '*.'", func() {
			Expect(unescape(`\052.example.org.`)).To(Equal("*.example.org."))
		})

		It("leaves names without the escape prefix unchanged", func() {
			Expect(unescape("foo.example.org.")).To(Equal("foo.example.org."))
		})

		It("only matches the exact prefix, not '\\052' embedded later", func() {
			Expect(unescape(`a.\052.example.org.`)).To(Equal(`a.\052.example.org.`))
		})
	})

	Describe("decodeValue", func() {
		It("strips the surrounding quotes from a TXT value", func() {
			Expect(decodeValue(`"hello"`, dns.TypeTXT)).To(Equal("hello"))
		})

		It("preserves an unquotable TXT value as-is", func() {
			// strconv.Unquote rejects an unbalanced string; the helper falls back to the raw value.
			Expect(decodeValue(`hello`, dns.TypeTXT)).To(Equal("hello"))
		})

		It("does not unquote non-TXT records", func() {
			Expect(decodeValue(`"1.2.3.4"`, dns.TypeA)).To(Equal(`"1.2.3.4"`))
		})

		It("decodes escape sequences inside the quoted TXT value", func() {
			Expect(decodeValue(`"a\"b"`, dns.TypeTXT)).To(Equal(`a"b`))
		})
	})

	Describe("queryDNS", func() {
		zoneInfo := dns.NewZoneInfo(dns.NewZoneID(ProviderType, "Z1"), "example.org", true, "/hostedzone/Z1")

		It("matches a wildcard TXT and unquotes its value", func() {
			fake.listResourceRecordsFn = func(_ context.Context, _ *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
				return &route53.ListResourceRecordSetsOutput{
					ResourceRecordSets: []route53types.ResourceRecordSet{
						{
							Name:            aws.String(`\052.example.org.`),
							Type:            route53types.RRTypeTxt,
							TTL:             aws.Int64(120),
							ResourceRecords: []route53types.ResourceRecord{{Value: aws.String(`"hello"`)}, {Value: aws.String(`"world"`)}},
						},
					},
				}, nil
			}
			h := newTestHandler(fake)

			rs, err := h.queryDNS(ctx, zoneInfo, dns.DNSSetName{DNSName: "*.example.org."}, dns.TypeTXT)
			Expect(err).NotTo(HaveOccurred())
			Expect(rs).NotTo(BeNil())
			Expect(rs.Records).To(HaveLen(2))
			Expect(rs.Records[0].Value).To(Equal("hello"))
			Expect(rs.Records[1].Value).To(Equal("world"))
		})

		It("matches a wildcard A record and leaves its value untouched", func() {
			fake.listResourceRecordsFn = func(_ context.Context, _ *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
				return &route53.ListResourceRecordSetsOutput{
					ResourceRecordSets: []route53types.ResourceRecordSet{
						{
							Name:            aws.String(`\052.example.org.`),
							Type:            route53types.RRTypeA,
							TTL:             aws.Int64(60),
							ResourceRecords: []route53types.ResourceRecord{{Value: aws.String("1.2.3.4")}},
						},
					},
				}, nil
			}
			h := newTestHandler(fake)

			rs, err := h.queryDNS(ctx, zoneInfo, dns.DNSSetName{DNSName: "*.example.org."}, dns.TypeA)
			Expect(err).NotTo(HaveOccurred())
			Expect(rs).NotTo(BeNil())
			Expect(rs.Type).To(Equal(dns.TypeA))
			Expect(rs.TTL).To(Equal(int64(60)))
			Expect(rs.Records).To(HaveLen(1))
			Expect(rs.Records[0].Value).To(Equal("1.2.3.4"))
		})
	})
})
