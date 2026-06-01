// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var _ = Describe("DNSHandler", func() {
	var (
		ctx  context.Context
		fake *fakeRoute53
		h    *handler
	)

	BeforeEach(func() {
		ctx = logr.NewContext(context.Background(), logr.Discard())
		fake = &fakeRoute53{}
	})

	Describe("ProviderType", func() {
		It("returns the aws-route53 type", func() {
			h = newTestHandler(fake)
			Expect(h.ProviderType()).To(Equal(ProviderType))
			Expect(h.ProviderType()).To(Equal("aws-route53"))
		})
	})

	Describe("Release", func() {
		It("is a no-op", func() {
			h = newTestHandler(fake)
			Expect(func() { h.Release() }).NotTo(Panic())
		})
	})

	Describe("GetZones", func() {
		It("returns normalized hosted zones", func() {
			fake.listHostedZonesFn = func(_ context.Context, _ *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
				return &route53.ListHostedZonesOutput{
					HostedZones: []route53types.HostedZone{
						{Id: aws.String("/hostedzone/Z1"), Name: aws.String("example.org."), Config: &route53types.HostedZoneConfig{PrivateZone: false}},
						{Id: aws.String("/hostedzone/Z2"), Name: aws.String("private.example.com."), Config: &route53types.HostedZoneConfig{PrivateZone: true}},
					},
				}, nil
			}
			h = newTestHandler(fake)

			zones, err := h.GetZones(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(zones).To(HaveLen(2))
			Expect(zones[0].ZoneID().ID).To(Equal("Z1"))
			Expect(zones[0].Domain()).To(Equal("example.org"))
			Expect(zones[0].IsPrivate()).To(BeFalse())
			Expect(zones[0].Key()).To(Equal("/hostedzone/Z1"))
			Expect(zones[1].ZoneID().ID).To(Equal("Z2"))
			Expect(zones[1].IsPrivate()).To(BeTrue())
		})

		It("filters out blocked zones", func() {
			fake.listHostedZonesFn = func(_ context.Context, _ *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
				return &route53.ListHostedZonesOutput{
					HostedZones: []route53types.HostedZone{
						{Id: aws.String("/hostedzone/Z1"), Name: aws.String("example.org."), Config: &route53types.HostedZoneConfig{PrivateZone: false}},
						{Id: aws.String("/hostedzone/Z2"), Name: aws.String("blocked.example.com."), Config: &route53types.HostedZoneConfig{PrivateZone: false}},
					},
				}, nil
			}
			h = newTestHandler(fake, "Z2")

			zones, err := h.GetZones(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(zones).To(HaveLen(1))
			Expect(zones[0].ZoneID().ID).To(Equal("Z1"))
		})

		It("propagates errors", func() {
			fake.listHostedZonesFn = func(_ context.Context, _ *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
				return nil, errors.New("boom")
			}
			h = newTestHandler(fake)

			_, err := h.GetZones(ctx)
			Expect(err).To(MatchError(ContainSubstring("boom")))
		})

		It("returns an empty result when there are no zones", func() {
			fake.listHostedZonesFn = func(_ context.Context, _ *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
				return &route53.ListHostedZonesOutput{}, nil
			}
			h = newTestHandler(fake)

			zones, err := h.GetZones(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(zones).To(BeEmpty())
		})
	})

	Describe("GetCustomQueryDNSFunc", func() {
		zoneInfoPrivate := dns.NewZoneInfo(dns.NewZoneID(ProviderType, "Z1"), "example.org", true, "/hostedzone/Z1")
		zoneInfoPublic := dns.NewZoneInfo(dns.NewZoneID(ProviderType, "Z2"), "example.org", false, "/hostedzone/Z2")

		It("returns the handler's queryDNS for private zones (no factory call)", func() {
			h = newTestHandler(fake)
			factoryCalled := false
			factory := utils.QueryDNSFactoryFunc(func() (utils.QueryDNS, error) {
				factoryCalled = true
				return nil, nil
			})

			fn, err := h.GetCustomQueryDNSFunc(zoneInfoPrivate, factory)
			Expect(err).NotTo(HaveOccurred())
			Expect(fn).NotTo(BeNil())
			Expect(factoryCalled).To(BeFalse())

			fake.listResourceRecordsFn = func(_ context.Context, _ *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
				return &route53.ListResourceRecordSetsOutput{}, nil
			}
			rs, err := fn(ctx, zoneInfoPrivate, dns.DNSSetName{DNSName: "x.example.org"}, dns.TypeA)
			Expect(err).NotTo(HaveOccurred())
			Expect(rs).To(BeNil())
		})

		It("returns an error when the factory fails for a public zone", func() {
			h = newTestHandler(fake)
			factory := utils.QueryDNSFactoryFunc(func() (utils.QueryDNS, error) {
				return nil, errors.New("factory failed")
			})

			_, err := h.GetCustomQueryDNSFunc(zoneInfoPublic, factory)
			Expect(err).To(MatchError(ContainSubstring("factory failed")))
		})

		It("falls through to the default query function for plain types in public zones", func() {
			h = newTestHandler(fake)
			expected := dns.NewRecordSet(dns.TypeA, 60, []*dns.Record{{Value: "1.2.3.4"}})
			factory := utils.QueryDNSFactoryFunc(func() (utils.QueryDNS, error) {
				return &fakeQueryDNS{result: utils.QueryDNSResult{RecordSet: expected}}, nil
			})

			fn, err := h.GetCustomQueryDNSFunc(zoneInfoPublic, factory)
			Expect(err).NotTo(HaveOccurred())

			rs, err := fn(ctx, zoneInfoPublic, dns.DNSSetName{DNSName: "x.example.org"}, dns.TypeA)
			Expect(err).NotTo(HaveOccurred())
			Expect(rs).To(Equal(expected))
		})

		It("falls through to handler.queryDNS when a SetIdentifier is present (public zone)", func() {
			h = newTestHandler(fake)
			factoryQueriedTypes := []dns.RecordType{}
			factory := utils.QueryDNSFactoryFunc(func() (utils.QueryDNS, error) {
				return &fakeQueryDNS{
					queryFn: func(_ context.Context, _ dns.DNSSetName, t dns.RecordType) utils.QueryDNSResult {
						factoryQueriedTypes = append(factoryQueriedTypes, t)
						return utils.QueryDNSResult{}
					},
				}, nil
			})

			fn, err := h.GetCustomQueryDNSFunc(zoneInfoPublic, factory)
			Expect(err).NotTo(HaveOccurred())

			fake.listResourceRecordsFn = func(_ context.Context, _ *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
				return &route53.ListResourceRecordSetsOutput{}, nil
			}

			rs, err := fn(ctx, zoneInfoPublic, dns.DNSSetName{DNSName: "x.example.org", SetIdentifier: "id1"}, dns.TypeA)
			Expect(err).NotTo(HaveOccurred())
			Expect(rs).To(BeNil())
			Expect(factoryQueriedTypes).To(BeEmpty(), "factory must not be invoked when SetIdentifier is set")
		})

		It("synthesizes an alias record from a TXT lookup for ALIAS_A in public zones", func() {
			h = newTestHandler(fake)
			factory := utils.QueryDNSFactoryFunc(func() (utils.QueryDNS, error) {
				return &fakeQueryDNS{
					queryFn: func(_ context.Context, _ dns.DNSSetName, t dns.RecordType) utils.QueryDNSResult {
						switch t {
						case dns.TypeA:
							return utils.QueryDNSResult{RecordSet: dns.NewRecordSet(dns.TypeA, 60, []*dns.Record{{Value: "1.2.3.4"}})}
						case dns.TypeTXT:
							return utils.QueryDNSResult{RecordSet: dns.NewRecordSet(dns.TypeTXT, 60, []*dns.Record{{Value: "alias-target.elb.amazonaws.com."}})}
						}
						return utils.QueryDNSResult{Err: fmt.Errorf("unexpected type %s", t)}
					},
				}, nil
			})

			fn, err := h.GetCustomQueryDNSFunc(zoneInfoPublic, factory)
			Expect(err).NotTo(HaveOccurred())

			rs, err := fn(ctx, zoneInfoPublic, dns.DNSSetName{DNSName: "alias.example.org"}, dns.TypeAWS_ALIAS_A)
			Expect(err).NotTo(HaveOccurred())
			Expect(rs).NotTo(BeNil())
			Expect(rs.Type).To(Equal(dns.TypeAWS_ALIAS_A))
			Expect(rs.TTL).To(BeZero())
			Expect(rs.Records).To(HaveLen(1))
			Expect(rs.Records[0].Value).To(Equal("alias-target.elb.amazonaws.com"))
		})

		It("returns nil when the alias TXT lookup yields no record", func() {
			h = newTestHandler(fake)
			factory := utils.QueryDNSFactoryFunc(func() (utils.QueryDNS, error) {
				return &fakeQueryDNS{
					queryFn: func(_ context.Context, _ dns.DNSSetName, t dns.RecordType) utils.QueryDNSResult {
						if t == dns.TypeAAAA {
							return utils.QueryDNSResult{RecordSet: dns.NewRecordSet(dns.TypeAAAA, 60, []*dns.Record{{Value: "fe80::1"}})}
						}
						return utils.QueryDNSResult{}
					},
				}, nil
			})

			fn, err := h.GetCustomQueryDNSFunc(zoneInfoPublic, factory)
			Expect(err).NotTo(HaveOccurred())

			rs, err := fn(ctx, zoneInfoPublic, dns.DNSSetName{DNSName: "alias6.example.org"}, dns.TypeAWS_ALIAS_AAAA)
			Expect(err).NotTo(HaveOccurred())
			Expect(rs).To(BeNil())
		})

		It("returns the IP query error from the alias path", func() {
			h = newTestHandler(fake)
			factory := utils.QueryDNSFactoryFunc(func() (utils.QueryDNS, error) {
				return &fakeQueryDNS{
					queryFn: func(_ context.Context, _ dns.DNSSetName, _ dns.RecordType) utils.QueryDNSResult {
						return utils.QueryDNSResult{Err: errors.New("ip lookup failed")}
					},
				}, nil
			})

			fn, err := h.GetCustomQueryDNSFunc(zoneInfoPublic, factory)
			Expect(err).NotTo(HaveOccurred())

			_, qErr := fn(ctx, zoneInfoPublic, dns.DNSSetName{DNSName: "alias.example.org"}, dns.TypeAWS_ALIAS_A)
			Expect(qErr).To(MatchError(ContainSubstring("ip lookup failed")))
		})
	})

	Describe("ExecuteRequests", func() {
		zone := provider.NewDNSHostedZone(ProviderType, "Z1", "example.org", "/hostedzone/Z1", false)

		It("creates one Create change for a new record set", func() {
			h = newTestHandler(fake)
			reqs := provider.ChangeRequests{
				Name: dns.DNSSetName{DNSName: "x.example.org"},
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {New: dns.NewRecordSet(dns.TypeA, 60, []*dns.Record{{Value: "1.2.3.4"}})},
				},
			}

			Expect(h.ExecuteRequests(ctx, zone, reqs)).To(Succeed())
			Expect(fake.changeCalls).To(HaveLen(1))
			batch := fake.changeCalls[0].ChangeBatch
			Expect(batch.Changes).To(HaveLen(1))
			Expect(batch.Changes[0].Action).To(Equal(route53types.ChangeActionCreate))
			Expect(aws.ToString(batch.Changes[0].ResourceRecordSet.Name)).To(Equal("x.example.org."))
			Expect(batch.Changes[0].ResourceRecordSet.Type).To(Equal(route53types.RRTypeA))
		})

		It("emits a Delete change when only Old is provided", func() {
			h = newTestHandler(fake)
			reqs := provider.ChangeRequests{
				Name: dns.DNSSetName{DNSName: "x.example.org"},
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {Old: dns.NewRecordSet(dns.TypeA, 60, []*dns.Record{{Value: "1.2.3.4"}})},
				},
			}

			Expect(h.ExecuteRequests(ctx, zone, reqs)).To(Succeed())
			Expect(fake.changeCalls).To(HaveLen(1))
			batch := fake.changeCalls[0].ChangeBatch
			Expect(batch.Changes).To(HaveLen(1))
			Expect(batch.Changes[0].Action).To(Equal(route53types.ChangeActionDelete))
		})

		It("emits an Upsert change when Old and New share the same type", func() {
			h = newTestHandler(fake)
			reqs := provider.ChangeRequests{
				Name: dns.DNSSetName{DNSName: "x.example.org"},
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {
						Old: dns.NewRecordSet(dns.TypeA, 60, []*dns.Record{{Value: "1.2.3.4"}}),
						New: dns.NewRecordSet(dns.TypeA, 120, []*dns.Record{{Value: "1.2.3.5"}}),
					},
				},
			}

			Expect(h.ExecuteRequests(ctx, zone, reqs)).To(Succeed())
			Expect(fake.changeCalls).To(HaveLen(1))
			Expect(fake.changeCalls[0].ChangeBatch.Changes).To(HaveLen(1))
			Expect(fake.changeCalls[0].ChangeBatch.Changes[0].Action).To(Equal(route53types.ChangeActionUpsert))
			Expect(aws.ToInt64(fake.changeCalls[0].ChangeBatch.Changes[0].ResourceRecordSet.TTL)).To(Equal(int64(120)))
		})

		It("splits a type change into Delete + Create", func() {
			h = newTestHandler(fake)
			reqs := provider.ChangeRequests{
				Name: dns.DNSSetName{DNSName: "x.example.org"},
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {
						Old: dns.NewRecordSet(dns.TypeA, 60, []*dns.Record{{Value: "1.2.3.4"}}),
						New: dns.NewRecordSet(dns.TypeCNAME, 60, []*dns.Record{{Value: "target.example.org."}}),
					},
				},
			}

			Expect(h.ExecuteRequests(ctx, zone, reqs)).To(Succeed())
			// Deletes go in their own batch first; creates follow in the next batch.
			Expect(fake.changeCalls).To(HaveLen(2))
			Expect(fake.changeCalls[0].ChangeBatch.Changes).To(HaveLen(1))
			Expect(fake.changeCalls[0].ChangeBatch.Changes[0].Action).To(Equal(route53types.ChangeActionDelete))
			Expect(fake.changeCalls[1].ChangeBatch.Changes).To(HaveLen(1))
			Expect(fake.changeCalls[1].ChangeBatch.Changes[0].Action).To(Equal(route53types.ChangeActionCreate))
			Expect(fake.changeCalls[1].ChangeBatch.Changes[0].ResourceRecordSet.Type).To(Equal(route53types.RRTypeCname))
		})

		It("reports an error when both Old and New are nil", func() {
			h = newTestHandler(fake)
			reqs := provider.ChangeRequests{
				Name: dns.DNSSetName{DNSName: "x.example.org"},
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {},
				},
			}

			err := h.ExecuteRequests(ctx, zone, reqs)
			Expect(err).To(MatchError(ContainSubstring("both old and new record sets are nil")))
			Expect(fake.changeCalls).To(BeEmpty())
		})

		It("does nothing when there are no updates", func() {
			h = newTestHandler(fake)
			reqs := provider.ChangeRequests{
				Name:    dns.DNSSetName{DNSName: "x.example.org"},
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{},
			}

			Expect(h.ExecuteRequests(ctx, zone, reqs)).To(Succeed())
			Expect(fake.changeCalls).To(BeEmpty())
		})

		It("propagates submission errors", func() {
			h = newTestHandler(fake)
			fake.changeResourceRecordsFn = func(_ context.Context, _ *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
				return nil, errors.New("server error")
			}
			reqs := provider.ChangeRequests{
				Name: dns.DNSSetName{DNSName: "x.example.org"},
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {New: dns.NewRecordSet(dns.TypeA, 60, []*dns.Record{{Value: "1.2.3.4"}})},
				},
			}

			err := h.ExecuteRequests(ctx, zone, reqs)
			Expect(err).To(MatchError(ContainSubstring("changes failed")))
		})
	})
})

// fakeQueryDNS is a controllable utils.QueryDNS used by GetCustomQueryDNSFunc tests.
type fakeQueryDNS struct {
	result  utils.QueryDNSResult
	queryFn func(ctx context.Context, setName dns.DNSSetName, recordType dns.RecordType) utils.QueryDNSResult
}

func (q *fakeQueryDNS) Query(ctx context.Context, setName dns.DNSSetName, recordType dns.RecordType) utils.QueryDNSResult {
	if q.queryFn != nil {
		return q.queryFn(ctx, setName, recordType)
	}
	return q.result
}
