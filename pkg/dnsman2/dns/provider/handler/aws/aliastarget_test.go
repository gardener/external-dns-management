// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("aliastarget", func() {
	Describe("buildRecordSetFromAliasTarget", func() {
		It("returns nil when there is no alias target", func() {
			Expect(buildRecordSetFromAliasTarget(route53types.ResourceRecordSet{Type: route53types.RRTypeA})).To(BeNil())
		})

		It("returns nil for an unsupported record type", func() {
			r := route53types.ResourceRecordSet{
				Type:        route53types.RRTypeCname,
				AliasTarget: &route53types.AliasTarget{DNSName: aws.String("foo.us-east-1.elb.amazonaws.com")},
			}
			Expect(buildRecordSetFromAliasTarget(r)).To(BeNil())
		})

		It("maps an A alias target to an ALIAS_A record set with normalized value and zero TTL", func() {
			r := route53types.ResourceRecordSet{
				Type:        route53types.RRTypeA,
				AliasTarget: &route53types.AliasTarget{DNSName: aws.String("Foo.us-east-1.elb.amazonaws.com.")},
			}
			rs := buildRecordSetFromAliasTarget(r)
			Expect(rs).NotTo(BeNil())
			Expect(rs.Type).To(Equal(dns.TypeAWS_ALIAS_A))
			Expect(rs.TTL).To(Equal(int64(0)))
			Expect(rs.Records).To(HaveLen(1))
			Expect(rs.Records[0].Value).To(Equal("foo.us-east-1.elb.amazonaws.com"))
		})

		It("maps an AAAA alias target to an ALIAS_AAAA record set", func() {
			r := route53types.ResourceRecordSet{
				Type:        route53types.RRTypeAaaa,
				AliasTarget: &route53types.AliasTarget{DNSName: aws.String("foo.us-east-1.elb.amazonaws.com")},
			}
			rs := buildRecordSetFromAliasTarget(r)
			Expect(rs).NotTo(BeNil())
			Expect(rs.Type).To(Equal(dns.TypeAWS_ALIAS_AAAA))
			Expect(rs.Records[0].Value).To(Equal("foo.us-east-1.elb.amazonaws.com"))
		})
	})

	Describe("buildResourceRecordSetForAliasTarget", func() {
		var (
			ctx           context.Context
			policyContext *routingPolicyContext
			name          dns.DNSSetName
		)

		BeforeEach(func() {
			ctx = context.Background()
			policyContext = newRoutingPolicyContext(&fakeRoute53{})
			name = dns.DNSSetName{DNSName: "foo.example.com"}
		})

		aliasRecordSet := func(rtype dns.RecordType, target string) *dns.RecordSet {
			return dns.NewRecordSet(rtype, 0, []*dns.Record{{Value: target}})
		}

		It("returns nil for a non-alias record set", func() {
			rrset, err := buildResourceRecordSetForAliasTarget(ctx, name, nil, policyContext, dns.NewRecordSet(dns.TypeA, 300, []*dns.Record{{Value: "1.2.3.4"}}), false)
			Expect(err).NotTo(HaveOccurred())
			Expect(rrset).To(BeNil())
		})

		It("builds an A alias target for an enabled canonical hosted zone", func() {
			rrset, err := buildResourceRecordSetForAliasTarget(ctx, name, nil, policyContext, aliasRecordSet(dns.TypeAWS_ALIAS_A, "bar.us-east-1.elb.amazonaws.com"), false)
			Expect(err).NotTo(HaveOccurred())
			Expect(rrset).NotTo(BeNil())
			Expect(rrset.Type).To(Equal(route53types.RRTypeA))
			Expect(aws.ToString(rrset.Name)).To(Equal("foo.example.com"))
			Expect(rrset.AliasTarget).NotTo(BeNil())
			Expect(aws.ToString(rrset.AliasTarget.DNSName)).To(Equal("bar.us-east-1.elb.amazonaws.com"))
			Expect(aws.ToString(rrset.AliasTarget.HostedZoneId)).To(Equal("Z35SXDOTRQ7X7K"))
			Expect(rrset.AliasTarget.EvaluateTargetHealth).To(BeTrue())
		})

		It("builds an AAAA alias target", func() {
			rrset, err := buildResourceRecordSetForAliasTarget(ctx, name, nil, policyContext, aliasRecordSet(dns.TypeAWS_ALIAS_AAAA, "bar.us-east-1.elb.amazonaws.com"), false)
			Expect(err).NotTo(HaveOccurred())
			Expect(rrset.Type).To(Equal(route53types.RRTypeAaaa))
		})

		It("fails for an unknown hosted zone when not deleting", func() {
			_, err := buildResourceRecordSetForAliasTarget(ctx, name, nil, policyContext, aliasRecordSet(dns.TypeAWS_ALIAS_A, "bar.unknown.example.com"), false)
			Expect(err).To(MatchError(ContainSubstring("corrupted alias record set")))
		})

		It("resolves a disabled canonical hosted zone only when deleting", func() {
			target := "bar.us-gov-west-1.elb.amazonaws.com"

			_, err := buildResourceRecordSetForAliasTarget(ctx, name, nil, policyContext, aliasRecordSet(dns.TypeAWS_ALIAS_A, target), false)
			Expect(err).To(MatchError(ContainSubstring("corrupted alias record set")))

			rrset, err := buildResourceRecordSetForAliasTarget(ctx, name, nil, policyContext, aliasRecordSet(dns.TypeAWS_ALIAS_A, target), true)
			Expect(err).NotTo(HaveOccurred())
			Expect(aws.ToString(rrset.AliasTarget.HostedZoneId)).To(Equal("Z33AYJ8TM3BH4J"))
		})
	})
})
