// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package google

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	googledns "google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

type namedRecordSet struct {
	name dns.DNSSetName
	rs   *dns.RecordSet
}

var _ = Describe("execution", func() {
	var (
		nameFunc = func(element any) string {
			return element.(*googledns.ResourceRecordSet).Name
		}

		wrrStatus0 = func(name string, typ dns.RecordType) (*googledns.ResourceRecordSet, error) {
			switch name {
			case "w1.example.org.":
				return nil, &googleapi.Error{Code: 404}
			case "w2.example.org.":
				if typ == dns.TypeCNAME {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Wrr: &googledns.RRSetRoutingPolicyWrrPolicy{
								Items: []*googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{
									{
										Rrdatas: []string{"some-other.example.org."},
										Weight:  1,
									},
									{
										Rrdatas: []string{rrDefaultValue(typ)},
										Weight:  0,
									},
									{
										Rrdatas: []string{"some.example.org."},
										Weight:  1,
									},
								},
							},
						},
					}, nil
				}
			case "w3.example.org.":
				if typ == dns.TypeTXT {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Wrr: &googledns.RRSetRoutingPolicyWrrPolicy{
								Items: []*googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{
									{
										Rrdatas: []string{rrDefaultValue(typ)},
										Weight:  0,
									},
									{
										Rrdatas: []string{"\"bla\"", "\"foo\""},
										Weight:  1,
									},
								},
							},
						},
					}, nil
				}
			}
			return nil, fmt.Errorf("unexpected: %s %s", name, typ)
		}
		wrrStatus1 = func(name string, typ dns.RecordType) (*googledns.ResourceRecordSet, error) {
			switch name {
			case "w1.example.org.":
				if typ == dns.TypeA {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Wrr: &googledns.RRSetRoutingPolicyWrrPolicy{
								Items: []*googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{
									{
										Rrdatas: []string{"4.4.4.4"},
										Weight:  4,
									},
								},
							},
						},
					}, nil
				}
			case "w2.example.org.":
				if typ == dns.TypeCNAME {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Wrr: &googledns.RRSetRoutingPolicyWrrPolicy{
								Items: []*googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{
									{
										Rrdatas: []string{rrDefaultValue(typ)},
										Weight:  0,
									},
									{
										Rrdatas: []string{rrDefaultValue(typ)},
										Weight:  0,
									},
									{
										Rrdatas: []string{"some.example.org."},
										Weight:  1,
									},
								},
							},
						},
					}, nil
				}
			case "w3.example.org.":
				if typ == dns.TypeTXT {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Wrr: &googledns.RRSetRoutingPolicyWrrPolicy{
								Items: []*googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{
									{
										Rrdatas: []string{"\"bar\""},
										Weight:  5,
									},
									{
										Rrdatas: []string{"\"bla\"", "\"foo\""},
										Weight:  1,
									},
								},
							},
						},
					}, nil
				}
			}
			return nil, fmt.Errorf("unexpected: %s %s", name, typ)
		}
		wrrStatus2 = func(name string, typ dns.RecordType) (*googledns.ResourceRecordSet, error) {
			switch name {
			case "w1.example.org.":
				if typ == dns.TypeA {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Wrr: &googledns.RRSetRoutingPolicyWrrPolicy{
								Items: []*googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{
									{
										Rrdatas: []string{"4.4.4.4"},
										Weight:  4,
									},
									{
										Rrdatas: []string{rrDefaultValue(typ)},
										Weight:  0,
									},
									{
										Rrdatas: []string{"5.5.5.5"},
										Weight:  5,
									},
								},
							},
						},
					}, nil
				}
			case "w4.example.org.":
				if typ == dns.TypeAAAA {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Wrr: &googledns.RRSetRoutingPolicyWrrPolicy{
								Items: []*googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{
									{
										Rrdatas: []string{rrDefaultValue(typ)},
										Weight:  0,
									},
									{
										Rrdatas: []string{rrDefaultValue(typ)},
										Weight:  0,
									},
									{
										Rrdatas: []string{"cef::1"},
										Weight:  1,
									},
								},
							},
						},
					}, nil
				}
			}
			return nil, fmt.Errorf("unexpected: %s %s", name, typ)
		}
		rs1    = makeNamedRecordSet("x1.example.org", dns.TypeA, 301, "1.1.1.1")
		rs2old = makeNamedRecordSet("x2.example.org", dns.TypeA, 302, "1.1.1.2")
		rs2new = makeNamedRecordSet("x2.example.org", dns.TypeA, 303, "1.1.1.3")
		rs4    = makeNamedRecordSet("x4.example.org", dns.TypeA, 304, "1.1.1.4")

		rsWrr1_0     = makePairWrr("w1.example.org", 0, 10, dns.TypeA, "1.1.2.0")
		rsWrr1_2     = makePairWrr("w1.example.org", 2, 12, dns.TypeA, "1.1.2.2")
		rsWrr2_2old  = makePairWrr("w2.example.org", 2, 1, dns.TypeCNAME, "some.example.org")
		rsWrr2_2new  = makePairWrr("w2.example.org", 2, 0, dns.TypeCNAME, "some.example.org")
		rsWrr2_0new  = makePairWrr("w2.example.org", 0, 0, dns.TypeCNAME, "some.example.org")
		rsWrr3_1     = makePairWrr("w3.example.org", 1, 1, dns.TypeTXT, "bla", "foo")
		rsWrr4_0     = makePairWrr("w4.example.org", 0, 1, dns.TypeAAAA, "a23::4")
		rsWrr1_0b    = makePairWrr("w1.example.org", 0, 4, dns.TypeA, "4.4.4.4")
		rsWrr1_2b    = makePairWrr("w1.example.org", 2, 5, dns.TypeA, "5.5.5.5")
		rsWrr1_2bnew = makePairWrr("w1.example.org", 2, 0, dns.TypeA, "5.5.6.6")

		geoStatus0 = func(name string, typ dns.RecordType) (*googledns.ResourceRecordSet, error) {
			switch name {
			case "w1.example.org.":
				return nil, &googleapi.Error{Code: 404}
			case "w2.example.org.":
				if typ == dns.TypeCNAME {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Geo: &googledns.RRSetRoutingPolicyGeoPolicy{
								Items: []*googledns.RRSetRoutingPolicyGeoPolicyGeoPolicyItem{
									{
										Rrdatas:  []string{"some-other.example.org."},
										Location: "asia-east2",
									},
									{
										Rrdatas:  []string{"some.example.org."},
										Location: "europe-west1",
									},
								},
							},
						},
					}, nil
				}
			case "w3.example.org.":
				if typ == dns.TypeTXT {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Geo: &googledns.RRSetRoutingPolicyGeoPolicy{
								Items: []*googledns.RRSetRoutingPolicyGeoPolicyGeoPolicyItem{
									{
										Rrdatas:  []string{"\"bla\"", "\"foo\""},
										Location: "asia-east2",
									},
								},
							},
						},
					}, nil
				}
			}
			return nil, fmt.Errorf("unexpected: %s %s", name, typ)
		}
		geoStatus0b = func(name string, typ dns.RecordType) (*googledns.ResourceRecordSet, error) {
			switch name {
			case "w1.example.org.":
				if typ == dns.TypeA {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Geo: &googledns.RRSetRoutingPolicyGeoPolicy{
								Items: []*googledns.RRSetRoutingPolicyGeoPolicyGeoPolicyItem{
									{
										Rrdatas:  []string{"1.1.2.0"},
										Location: "europe-west1",
									},
								},
							},
						},
					}, nil
				}
			}
			return geoStatus0(name, typ)
		}
		geoStatus1 = func(name string, typ dns.RecordType) (*googledns.ResourceRecordSet, error) {
			switch name {
			case "w1.example.org.":
				if typ == dns.TypeA {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Geo: &googledns.RRSetRoutingPolicyGeoPolicy{
								Items: []*googledns.RRSetRoutingPolicyGeoPolicyGeoPolicyItem{
									{
										Rrdatas:  []string{"1.1.2.1"},
										Location: "us-east1",
									},
									{
										Rrdatas:  []string{"1.1.2.2"},
										Location: "asia-east1",
									},
								},
							},
						},
					}, nil
				}
			case "w2.example.org.":
				if typ == dns.TypeCNAME {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: string(typ),
						RoutingPolicy: &googledns.RRSetRoutingPolicy{
							Geo: &googledns.RRSetRoutingPolicyGeoPolicy{
								Items: []*googledns.RRSetRoutingPolicyGeoPolicyGeoPolicyItem{
									{
										Rrdatas:  []string{"some-other.example.org."},
										Location: "asia-east2",
									},
									{
										Rrdatas:  []string{"some.example.org."},
										Location: "europe-west1",
									},
								},
							},
						},
					}, nil
				}
			}
			return nil, fmt.Errorf("unexpected: %s %s", name, typ)
		}
		rsGeo1_0    = makeNamedRecordSetGeo("w1.example.org", "europe-west1", dns.TypeA, "1.1.2.0")
		rsGeo1_2    = makeNamedRecordSetGeo("w1.example.org", "asia-east1", dns.TypeA, "1.1.2.2")
		rsGeo2_2old = makeNamedRecordSetGeo("w2.example.org", "europe-west1", dns.TypeCNAME, "some.example.org")
		rsGeo2_2new = makeNamedRecordSetGeo("w2.example.org", "europe-west1", dns.TypeCNAME, "some2.example.org")
		rsGeo3_1    = makeNamedRecordSetGeo("w3.example.org", "asia-east2", dns.TypeTXT, "bla", "foo")
	)
	DescribeTable("Should prepare submission", func(reqs provider.ChangeRequests, rrsetGetter rrsetGetterFunc, changeMatcher types.GomegaMatcher) {
		change, err := prepareSubmission(reqs, rrsetGetter)
		if changeMatcher == nil {
			Expect(err).To(HaveOccurred())
			return
		} else {
			Expect(err).To(Not(HaveOccurred()))
			Expect(change).To(PointTo(changeMatcher))
		}
	},
		Entry("fails for invalid index 5",
			newChangeRequestsBuilder(makeIndexedDNSSetName("w1.example.org", 5)).create(makeRecordSetWrr(1, dns.TypeA, "4.4.4.4")).build(),
			nil,
			nil,
		),
		Entry("fails for invalid weight 0.2",
			newChangeRequestsBuilder(makeIndexedDNSSetName("w1.example.org", 0)).create(makeRecordSetWrrWrongWeight02(dns.TypeA, "1.1.2.0")).build(),
			nil,
			nil,
		),
		Entry("fails for missing weight parameter",
			newChangeRequestsBuilder(makeIndexedDNSSetName("w1.example.org", 0)).create(makeRecordSetWrrMissingWeight(dns.TypeA, "1.1.2.0")).build(),
			nil,
			nil,
		),
		Entry("prepares simple non-policy-routing change requests - set1",
			newChangeRequestsBuilder(rs1.name).create(rs1.rs).build(),
			nil,
			MatchFields(IgnoreExtras, Fields{
				"Additions": MatchAllElements(nameFunc, Elements{
					"x1.example.org.": matchSimpleResourceRecordSet(dns.TypeA, 301, "1.1.1.1"),
				}),
				"Deletions": MatchAllElements(nameFunc, Elements{}),
			}),
		),
		Entry("prepares simple non-policy-routing change requests - set2",
			newChangeRequestsBuilder(rs2old.name).update(rs2old.rs, rs2new.rs).build(),
			nil,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"x2.example.org.": matchSimpleResourceRecordSet(dns.TypeA, 302, "1.1.1.2"),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"x2.example.org.": matchSimpleResourceRecordSet(dns.TypeA, 303, "1.1.1.3"),
				}),
			}),
		),
		Entry("prepares simple non-policy-routing change requests - set4",
			newChangeRequestsBuilder(rs4.name).delete(rs4.rs).build(),
			nil,
			MatchFields(IgnoreExtras, Fields{
				"Additions": MatchAllElements(nameFunc, Elements{}),
				"Deletions": MatchAllElements(nameFunc, Elements{
					"x4.example.org.": matchSimpleResourceRecordSet(dns.TypeA, 304, "1.1.1.4"),
				}),
			}),
		),
		Entry("prepares weighted policy-routing change requests - set1_0",
			newChangeRequestsBuilder(rsWrr1_0.name).create(rsWrr1_0.rs).build(),
			wrrStatus0,
			MatchFields(IgnoreExtras, Fields{
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.TypeA, matchWrrItem(10, "1.1.2.0")),
				}),
				"Deletions": MatchAllElements(nameFunc, Elements{}),
			}),
		),
		Entry("prepares weighted policy-routing change requests - set1_2",
			newChangeRequestsBuilder(rsWrr1_2.name).create(rsWrr1_2.rs).build(),
			wrrStatus0,
			MatchFields(IgnoreExtras, Fields{
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.TypeA, matchWrrPlaceholderItem(dns.TypeA), matchWrrPlaceholderItem(dns.TypeA), matchWrrItem(12, "1.1.2.2")),
				}),
				"Deletions": MatchAllElements(nameFunc, Elements{}),
			}),
		),
		Entry("prepares weighted policy-routing change requests - set2",
			newChangeRequestsBuilder(rsWrr2_2old.name).update(rsWrr2_2old.rs, rsWrr2_2new.rs).build(),
			wrrStatus0,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w2.example.org.": matchWrrResourceRecordSet(dns.TypeCNAME, matchWrrItem(1, "some-other.example.org."), matchWrrPlaceholderItem(dns.TypeCNAME), matchWrrItem(1, "some.example.org.")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w2.example.org.": matchWrrResourceRecordSet(dns.TypeCNAME, matchWrrItem(1, "some-other.example.org."), matchWrrPlaceholderItem(dns.TypeCNAME), matchWrrItem(0, "some.example.org.")),
				}),
			}),
		),
		Entry("prepares weighted policy-routing change requests - set3",
			newChangeRequestsBuilder(rsWrr3_1.name).delete(rsWrr3_1.rs).build(),
			wrrStatus0,
			MatchFields(IgnoreExtras, Fields{
				"Additions": MatchAllElements(nameFunc, Elements{}),
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w3.example.org.": matchWrrResourceRecordSet(dns.TypeTXT, matchWrrPlaceholderItem(dns.TypeTXT), matchWrrItem(1, "\"bla\"", "\"foo\"")),
				}),
			}),
		),
		Entry("prepares weighted policy-routing change requests (merging) - set1",
			newChangeRequestsBuilder(rsWrr1_2.name).create(rsWrr1_2.rs).build(),
			wrrStatus1,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.TypeA, matchWrrItem(4, "4.4.4.4")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.TypeA, matchWrrItem(4, "4.4.4.4"), matchWrrPlaceholderItem(dns.TypeA), matchWrrItem(12, "1.1.2.2")),
				}),
			}),
		),
		Entry("prepares weighted policy-routing change requests (merging) - set2_0",
			newChangeRequestsBuilder(rsWrr2_0new.name).create(rsWrr2_0new.rs).build(),
			wrrStatus1,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w2.example.org.": matchWrrResourceRecordSet(dns.TypeCNAME, matchWrrPlaceholderItem(dns.TypeCNAME), matchWrrPlaceholderItem(dns.TypeCNAME), matchWrrItem(1, "some.example.org.")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w2.example.org.": matchWrrResourceRecordSet(dns.TypeCNAME, matchWrrItem(0, "some.example.org."), matchWrrPlaceholderItem(dns.TypeCNAME), matchWrrItem(1, "some.example.org.")),
				}),
			}),
		),
		Entry("prepares weighted policy-routing change requests (merging) - set2_2",
			newChangeRequestsBuilder(rsWrr2_2old.name).delete(rsWrr2_2old.rs).build(),
			wrrStatus1,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w2.example.org.": matchWrrResourceRecordSet(dns.TypeCNAME, matchWrrPlaceholderItem(dns.TypeCNAME), matchWrrPlaceholderItem(dns.TypeCNAME), matchWrrItem(1, "some.example.org.")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{}),
			}),
		),
		Entry("prepares weighted policy-routing change requests (merging) - set3",
			newChangeRequestsBuilder(rsWrr3_1.name).delete(rsWrr3_1.rs).build(),
			wrrStatus1,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w3.example.org.": matchWrrResourceRecordSet(dns.TypeTXT, matchWrrItem(5, "\"bar\""), matchWrrItem(1, "\"bla\"", "\"foo\"")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w3.example.org.": matchWrrResourceRecordSet(dns.TypeTXT, matchWrrItem(5, "\"bar\"")),
				}),
			}),
		),
		Entry("prepares weighted policy-routing change requests (merging2) - set1_0",
			newChangeRequestsBuilder(rsWrr1_0b.name).delete(rsWrr1_0b.rs).build(),
			wrrStatus2,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.TypeA, matchWrrItem(4, "4.4.4.4"), matchWrrPlaceholderItem(dns.TypeA), matchWrrItem(5, "5.5.5.5")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.TypeA, matchWrrPlaceholderItem(dns.TypeA), matchWrrPlaceholderItem(dns.TypeA), matchWrrItem(5, "5.5.5.5")),
				}),
			}),
		),
		Entry("prepares weighted policy-routing change requests (merging2) - set1_2",
			newChangeRequestsBuilder(rsWrr1_2b.name).update(rsWrr1_2b.rs, rsWrr1_2bnew.rs).build(),
			wrrStatus2,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.TypeA, matchWrrItem(4, "4.4.4.4"), matchWrrPlaceholderItem(dns.TypeA), matchWrrItem(5, "5.5.5.5")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.TypeA, matchWrrItem(4, "4.4.4.4"), matchWrrPlaceholderItem(dns.TypeA), matchWrrItem(0, "5.5.6.6")),
				}),
			}),
		),
		Entry("prepares weighted policy-routing change requests (merging2) - set4",
			newChangeRequestsBuilder(rsWrr4_0.name).create(rsWrr4_0.rs).build(),
			wrrStatus2,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w4.example.org.": matchWrrResourceRecordSet(dns.TypeAAAA, matchWrrPlaceholderItem(dns.TypeAAAA), matchWrrPlaceholderItem(dns.TypeAAAA), matchWrrItem(1, "cef::1")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w4.example.org.": matchWrrResourceRecordSet(dns.TypeAAAA, matchWrrItem(1, "a23::4"), matchWrrPlaceholderItem(dns.TypeAAAA), matchWrrItem(1, "cef::1")),
				}),
			}),
		),
		Entry("prepares geolocation policy-routing change requests - set1_0",
			newChangeRequestsBuilder(rsGeo1_0.name).create(rsGeo1_0.rs).build(),
			geoStatus0,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchGeoResourceRecordSet(dns.TypeA, matchGeoItem("europe-west1", "1.1.2.0")),
				}),
			}),
		),
		Entry("prepares geolocation policy-routing change requests - set1_2",
			newChangeRequestsBuilder(rsGeo1_2.name).create(rsGeo1_2.rs).build(),
			geoStatus0b,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchGeoResourceRecordSet(dns.TypeA, matchGeoItem("europe-west1", "1.1.2.0")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchGeoResourceRecordSet(dns.TypeA, matchGeoItem("europe-west1", "1.1.2.0"), matchGeoItem("asia-east1", "1.1.2.2")),
				}),
			}),
		),
		Entry("prepares geolocation policy-routing change requests - set2",
			newChangeRequestsBuilder(rsGeo2_2old.name).update(rsGeo2_2old.rs, rsGeo2_2new.rs).build(),
			geoStatus0,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w2.example.org.": matchGeoResourceRecordSet(dns.TypeCNAME, matchGeoItem("asia-east2", "some-other.example.org."), matchGeoItem("europe-west1", "some.example.org.")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w2.example.org.": matchGeoResourceRecordSet(dns.TypeCNAME, matchGeoItem("asia-east2", "some-other.example.org."), matchGeoItem("europe-west1", "some2.example.org.")),
				}),
			}),
		),
		Entry("prepares geolocation policy-routing change requests - set3",
			newChangeRequestsBuilder(rsGeo3_1.name).delete(rsGeo3_1.rs).build(),
			geoStatus0,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w3.example.org.": matchGeoResourceRecordSet(dns.TypeTXT, matchGeoItem("asia-east2", "\"bla\"", "\"foo\"")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{}),
			}),
		),
		Entry("prepares geolocation policy-routing change requests (merging) - set1_0",
			newChangeRequestsBuilder(rsGeo1_0.name).create(rsGeo1_0.rs).build(),
			geoStatus1,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchGeoResourceRecordSet(dns.TypeA, matchGeoItem("us-east1", "1.1.2.1"), matchGeoItem("asia-east1", "1.1.2.2")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchGeoResourceRecordSet(dns.TypeA, matchGeoItem("europe-west1", "1.1.2.0"), matchGeoItem("us-east1", "1.1.2.1"), matchGeoItem("asia-east1", "1.1.2.2")),
				}),
			}),
		),
		Entry("prepares geolocation policy-routing change requests (merging) - set1_2",
			newChangeRequestsBuilder(rsGeo1_2.name).create(rsGeo1_2.rs).build(),
			geoStatus1,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchGeoResourceRecordSet(dns.TypeA, matchGeoItem("us-east1", "1.1.2.1"), matchGeoItem("asia-east1", "1.1.2.2")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchGeoResourceRecordSet(dns.TypeA, matchGeoItem("us-east1", "1.1.2.1"), matchGeoItem("asia-east1", "1.1.2.2")),
				}),
			}),
		),
	)
})

func makeNamedRecordSet(dnsName string, typ dns.RecordType, ttl int64, targets ...string) namedRecordSet {
	set := dns.NewDNSSet(dns.DNSSetName{DNSName: dnsName})
	set.SetRecordSet(typ, nil, ttl, targets...)
	return namedRecordSet{name: set.Name, rs: set.Sets[typ]}
}

func makePairWrr(dnsName string, index, weight int, typ dns.RecordType, targets ...string) namedRecordSet {
	name := makeIndexedDNSSetName(dnsName, index)
	rs := makeRecordSetWrr(weight, typ, targets...)
	return namedRecordSet{name: name, rs: rs}
}

func makeRecordSetWrr(weight int, typ dns.RecordType, targets ...string) *dns.RecordSet {
	policy := &dns.RoutingPolicy{
		Type:       dns.RoutingPolicyWeighted,
		Parameters: map[string]string{"weight": fmt.Sprintf("%d", weight)},
	}
	set := dns.NewDNSSet(dns.DNSSetName{DNSName: "foo", SetIdentifier: "0"})
	set.SetRecordSet(typ, policy, 300, targets...)
	return set.Sets[typ]
}

func makeRecordSetWrrWrongWeight02(typ dns.RecordType, targets ...string) *dns.RecordSet {
	rs := makeRecordSetWrr(0, typ, targets...)
	rs.RoutingPolicy.Parameters["weight"] = "0.2"
	return rs
}

func makeRecordSetWrrMissingWeight(typ dns.RecordType, targets ...string) *dns.RecordSet {
	rs := makeRecordSetWrr(0, typ, targets...)
	delete(rs.RoutingPolicy.Parameters, "weight")
	return rs
}

func makeNamedRecordSetGeo(dnsName string, location string, typ dns.RecordType, targets ...string) namedRecordSet {
	policy := &dns.RoutingPolicy{
		Type:       dns.RoutingPolicyGeoLocation,
		Parameters: map[string]string{"location": location},
	}
	set := dns.NewDNSSet(dns.DNSSetName{DNSName: dnsName, SetIdentifier: location})
	set.SetRecordSet(typ, policy, 300, targets...)
	return namedRecordSet{name: set.Name, rs: set.Sets[typ]}
}

func matchSimpleResourceRecordSet(typ dns.RecordType, ttl int64, targets ...string) types.GomegaMatcher {
	return PointTo(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(string(typ)),
		"Ttl":     Equal(ttl),
		"Rrdatas": Equal(targets),
	}))
}

func itemNameFunc(index int, _ any) string {
	return fmt.Sprintf("%d", index)
}

func geoItemNameFunc(element any) string {
	if geo, ok := element.(*googledns.RRSetRoutingPolicyGeoPolicyGeoPolicyItem); ok {
		return geo.Location
	}
	return ""
}

func matchWrrResourceRecordSet(typ dns.RecordType, items ...types.GomegaMatcher) types.GomegaMatcher {
	elements := Elements{}
	for i, item := range items {
		elements[fmt.Sprintf("%d", i)] = item
	}

	return PointTo(MatchFields(IgnoreExtras, Fields{
		"Type": Equal(string(typ)),
		"RoutingPolicy": PointTo(MatchFields(IgnoreExtras, Fields{
			"Wrr": PointTo(MatchFields(IgnoreExtras, Fields{
				"Items": MatchAllElementsWithIndex(itemNameFunc, elements),
			})),
		})),
	}))
}

func matchWrrItem(weight int, targets ...string) types.GomegaMatcher {
	return PointTo(MatchFields(IgnoreExtras, Fields{
		"Weight":  Equal(float64(weight)),
		"Rrdatas": Equal(targets),
	}))
}

func matchWrrPlaceholderItem(typ dns.RecordType) types.GomegaMatcher {
	return PointTo(MatchFields(IgnoreExtras, Fields{
		"Weight":  Equal(float64(0)),
		"Rrdatas": Equal([]string{rrDefaultValue(typ)}),
	}))
}

func matchGeoResourceRecordSet(typ dns.RecordType, items ...geoItem) types.GomegaMatcher {
	elements := Elements{}
	for _, item := range items {
		elements[item.location] = PointTo(MatchFields(IgnoreExtras, Fields{
			"Location": Equal(item.location),
			"Rrdatas":  Equal(item.targets),
		}))
	}

	return PointTo(MatchFields(IgnoreExtras, Fields{
		"Type": Equal(string(typ)),
		"RoutingPolicy": PointTo(MatchFields(IgnoreExtras, Fields{
			"Geo": PointTo(MatchFields(IgnoreExtras, Fields{
				"Items": MatchAllElements(geoItemNameFunc, elements),
			})),
		})),
	}))
}

type geoItem struct {
	location string
	targets  []string
}

func matchGeoItem(location string, targets ...string) geoItem {
	return geoItem{location: location, targets: targets}
}

func prepareSubmission(reqs provider.ChangeRequests, rrsetGetter rrsetGetterFunc) (*googledns.Change, error) {
	zone := provider.NewDNSHostedZone(ProviderType, "test", "example.org", "", false)
	exec := newExecution(logr.Discard(), nil, zone.ZoneID())
	var errs []error
	for _, r := range reqs.Updates {
		var err error
		if r.New == nil && r.Old == nil {
			err = fmt.Errorf("both old and new record sets are nil for %s", reqs.Name)
			if err != nil {
				errs = append(errs, err)
			}
		}
		if r.Old != nil {
			err = exec.addChange(deleteAction, reqs, r.Old)
			if err != nil {
				errs = append(errs, err)
			}
		}
		if r.New != nil {
			err = exec.addChange(createAction, reqs, r.New)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if err := exec.prepareSubmission(rrsetGetter); err != nil {
		return nil, err
	}
	return exec.change, nil
}

func makeIndexedDNSSetName(dnsName string, index int) dns.DNSSetName {
	return dns.DNSSetName{DNSName: dnsName, SetIdentifier: fmt.Sprintf("%d", index)}
}

type changeRequestBuilder struct {
	reqs provider.ChangeRequests
}

func newChangeRequestsBuilder(name dns.DNSSetName) *changeRequestBuilder {
	return &changeRequestBuilder{
		reqs: *provider.NewChangeRequests(name),
	}
}

func (b *changeRequestBuilder) create(rs *dns.RecordSet) *changeRequestBuilder {
	b.reqs.Updates[rs.Type] = &provider.ChangeRequestUpdate{New: rs}
	return b
}

func (b *changeRequestBuilder) update(old, new *dns.RecordSet) *changeRequestBuilder {
	if old.Type == new.Type {
		b.reqs.Updates[old.Type] = &provider.ChangeRequestUpdate{Old: old, New: new}
	} else {
		b.reqs.Updates[old.Type] = &provider.ChangeRequestUpdate{Old: old}
		b.reqs.Updates[new.Type] = &provider.ChangeRequestUpdate{New: new}
	}
	return b
}

func (b *changeRequestBuilder) delete(rs *dns.RecordSet) *changeRequestBuilder {
	b.reqs.Updates[rs.Type] = &provider.ChangeRequestUpdate{Old: rs}
	return b
}

func (b *changeRequestBuilder) build() provider.ChangeRequests {
	return b.reqs
}
