// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package google

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	googledns "google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

var _ = Describe("Execution", func() {
	var (
		nameFunc = func(element interface{}) string {
			return element.(*googledns.ResourceRecordSet).Name
		}

		wrrStatus0 = func(name, typ string) (*googledns.ResourceRecordSet, error) {
			switch name {
			case "w1.example.org.":
				return nil, &googleapi.Error{Code: 404}
			case "w2.example.org.":
				if typ == dns.RS_CNAME {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: typ,
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
				if typ == dns.RS_TXT {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: typ,
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
		wrrStatus1 = func(name, typ string) (*googledns.ResourceRecordSet, error) {
			switch name {
			case "w1.example.org.":
				if typ == dns.RS_A {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: typ,
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
				if typ == dns.RS_CNAME {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: typ,
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
				if typ == dns.RS_TXT {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: typ,
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
		wrrStatus2 = func(name, typ string) (*googledns.ResourceRecordSet, error) {
			switch name {
			case "w1.example.org.":
				if typ == dns.RS_A {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: typ,
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
				if typ == dns.RS_AAAA {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: typ,
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
		dnsset1    = makeDNSSet("x1.example.org", dns.RS_A, 301, "1.1.1.1")
		dnsset2old = makeDNSSet("x2.example.org", dns.RS_A, 302, "1.1.1.2")
		dnsset2new = makeDNSSet("x2.example.org", dns.RS_A, 303, "1.1.1.3")
		dnsset4    = makeDNSSet("x4.example.org", dns.RS_A, 304, "1.1.1.4")

		dnssetwrr1_0     = makeDNSSetWrr("w1.example.org", 0, 10, dns.RS_A, "1.1.2.0")
		dnssetwrr1_2     = makeDNSSetWrr("w1.example.org", 2, 12, dns.RS_A, "1.1.2.2")
		dnssetwrr2_2old  = makeDNSSetWrr("w2.example.org", 2, 1, dns.RS_CNAME, "some.example.org")
		dnssetwrr2_2new  = makeDNSSetWrr("w2.example.org", 2, 0, dns.RS_CNAME, "some.example.org")
		dnssetwrr2_0new  = makeDNSSetWrr("w2.example.org", 0, 0, dns.RS_CNAME, "some.example.org")
		dnssetwrr3_1     = makeDNSSetWrr("w3.example.org", 1, 1, dns.RS_TXT, "bla", "foo")
		dnssetwrr4_0     = makeDNSSetWrr("w4.example.org", 0, 1, dns.RS_AAAA, "a23::4")
		dnssetwrr1_0b    = makeDNSSetWrr("w1.example.org", 0, 4, dns.RS_A, "4.4.4.4")
		dnssetwrr1_2b    = makeDNSSetWrr("w1.example.org", 2, 5, dns.RS_A, "5.5.5.5")
		dnssetwrr1_2bnew = makeDNSSetWrr("w1.example.org", 2, 0, dns.RS_A, "5.5.6.6")

		geoStatus0 = func(name, typ string) (*googledns.ResourceRecordSet, error) {
			switch name {
			case "w1.example.org.":
				return nil, &googleapi.Error{Code: 404}
			case "w2.example.org.":
				if typ == dns.RS_CNAME {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: typ,
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
				if typ == dns.RS_TXT {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: typ,
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
		geoStatus1 = func(name, typ string) (*googledns.ResourceRecordSet, error) {
			switch name {
			case "w1.example.org.":
				if typ == dns.RS_A {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: typ,
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
				if typ == dns.RS_CNAME {
					return &googledns.ResourceRecordSet{
						Name: name,
						Type: typ,
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
		dnssetgeo1_0    = makeDNSSetGeo("w1.example.org", "europe-west1", dns.RS_A, "1.1.2.0")
		dnssetgeo1_2    = makeDNSSetGeo("w1.example.org", "asia-east1", dns.RS_A, "1.1.2.2")
		dnssetgeo2_2old = makeDNSSetGeo("w2.example.org", "europe-west1", dns.RS_CNAME, "some.example.org")
		dnssetgeo2_2new = makeDNSSetGeo("w2.example.org", "europe-west1", dns.RS_CNAME, "some2.example.org")
		dnssetgeo3_1    = makeDNSSetGeo("w3.example.org", "asia-east2", dns.RS_TXT, "bla", "foo")
	)
	DescribeTable("Should prepare submission", func(reqs []*provider.ChangeRequest, rrsetGetter rrsetGetterFunc, changeMatcher types.GomegaMatcher) {
		change, err := prepareSubmission(reqs, rrsetGetter)
		if changeMatcher == nil {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).To(Not(HaveOccurred()))
			Expect(change).To(PointTo(changeMatcher))
		}
	},
		Entry("fails for invalid index 5",
			[]*provider.ChangeRequest{
				{Action: provider.R_CREATE, Type: dns.RS_A, Addition: makeDNSSetWrr("w1.example.org", 5, 1, dns.RS_A, "4.4.4.4")},
			},
			nil,
			nil,
		),
		Entry("fails for invalid weight 0.2",
			[]*provider.ChangeRequest{
				{Action: provider.R_CREATE, Type: dns.RS_A, Addition: makeDNSSetWrrWrongWeight02("w1.example.org", 0, dns.RS_A, "1.1.2.0")},
			},
			nil,
			nil,
		),
		Entry("fails for missing weight parameter",
			[]*provider.ChangeRequest{
				{Action: provider.R_CREATE, Type: dns.RS_A, Addition: makeDNSSetWrrMissingWeight("w1.example.org", 0, dns.RS_A, "1.1.2.0")},
			},
			nil,
			nil,
		),
		Entry("prepares simple non-policy-routing change requests",
			[]*provider.ChangeRequest{
				{Action: provider.R_CREATE, Type: dns.RS_A, Addition: dnsset1},
				{Action: provider.R_UPDATE, Type: dns.RS_A, Addition: dnsset2new, Deletion: dnsset2old},
				{Action: provider.R_DELETE, Type: dns.RS_A, Deletion: dnsset4},
			},
			nil,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"x2.example.org.": matchSimpleResourceRecordSet(dns.RS_A, 302, "1.1.1.2"),
					"x4.example.org.": matchSimpleResourceRecordSet(dns.RS_A, 304, "1.1.1.4"),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"x1.example.org.": matchSimpleResourceRecordSet(dns.RS_A, 301, "1.1.1.1"),
					"x2.example.org.": matchSimpleResourceRecordSet(dns.RS_A, 303, "1.1.1.3"),
				}),
			}),
		),
		Entry("prepares weighted policy-routing change requests",
			[]*provider.ChangeRequest{
				{Action: provider.R_CREATE, Type: dns.RS_A, Addition: dnssetwrr1_0},
				{Action: provider.R_CREATE, Type: dns.RS_A, Addition: dnssetwrr1_2},
				{Action: provider.R_UPDATE, Type: dns.RS_CNAME, Addition: dnssetwrr2_2new, Deletion: dnssetwrr2_2old},
				{Action: provider.R_DELETE, Type: dns.RS_TXT, Deletion: dnssetwrr3_1},
			},
			wrrStatus0,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w2.example.org.": matchWrrResourceRecordSet(dns.RS_CNAME, matchWrrItem(1, "some-other.example.org."), matchWrrPlaceholderItem(dns.RS_CNAME), matchWrrItem(1, "some.example.org.")),
					"w3.example.org.": matchWrrResourceRecordSet(dns.RS_TXT, matchWrrPlaceholderItem(dns.RS_TXT), matchWrrItem(1, "\"bla\"", "\"foo\"")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.RS_A, matchWrrItem(10, "1.1.2.0"), matchWrrPlaceholderItem(dns.RS_A), matchWrrItem(12, "1.1.2.2")),
					"w2.example.org.": matchWrrResourceRecordSet(dns.RS_CNAME, matchWrrItem(1, "some-other.example.org."), matchWrrPlaceholderItem(dns.RS_CNAME), matchWrrItem(0, "some.example.org.")),
				}),
			}),
		),
		Entry("prepares weighted policy-routing change requests (merging)",
			[]*provider.ChangeRequest{
				{Action: provider.R_CREATE, Type: dns.RS_A, Addition: dnssetwrr1_2},
				{Action: provider.R_DELETE, Type: dns.RS_CNAME, Deletion: dnssetwrr2_2old},
				{Action: provider.R_CREATE, Type: dns.RS_CNAME, Addition: dnssetwrr2_0new},
				{Action: provider.R_DELETE, Type: dns.RS_TXT, Deletion: dnssetwrr3_1},
			},
			wrrStatus1,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.RS_A, matchWrrItem(4, "4.4.4.4")),
					"w2.example.org.": matchWrrResourceRecordSet(dns.RS_CNAME, matchWrrPlaceholderItem(dns.RS_CNAME), matchWrrPlaceholderItem(dns.RS_CNAME), matchWrrItem(1, "some.example.org.")),
					"w3.example.org.": matchWrrResourceRecordSet(dns.RS_TXT, matchWrrItem(5, "\"bar\""), matchWrrItem(1, "\"bla\"", "\"foo\"")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.RS_A, matchWrrItem(4, "4.4.4.4"), matchWrrPlaceholderItem(dns.RS_A), matchWrrItem(12, "1.1.2.2")),
					"w2.example.org.": matchWrrResourceRecordSet(dns.RS_CNAME, matchWrrItem(0, "some.example.org.")),
					"w3.example.org.": matchWrrResourceRecordSet(dns.RS_TXT, matchWrrItem(5, "\"bar\"")),
				}),
			}),
		),
		Entry("prepares weighted policy-routing change requests (merging2)",
			[]*provider.ChangeRequest{
				{Action: provider.R_CREATE, Type: dns.RS_AAAA, Addition: dnssetwrr4_0},
				{Action: provider.R_DELETE, Type: dns.RS_A, Deletion: dnssetwrr1_0b},
				{Action: provider.R_UPDATE, Type: dns.RS_A, Addition: dnssetwrr1_2bnew, Deletion: dnssetwrr1_2b},
			},
			wrrStatus2,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.RS_A, matchWrrItem(4, "4.4.4.4"), matchWrrPlaceholderItem(dns.RS_A), matchWrrItem(5, "5.5.5.5")),
					"w4.example.org.": matchWrrResourceRecordSet(dns.RS_AAAA, matchWrrPlaceholderItem(dns.RS_AAAA), matchWrrPlaceholderItem(dns.RS_AAAA), matchWrrItem(1, "cef::1")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchWrrResourceRecordSet(dns.RS_A, matchWrrPlaceholderItem(dns.RS_A), matchWrrPlaceholderItem(dns.RS_A), matchWrrItem(0, "5.5.6.6")),
					"w4.example.org.": matchWrrResourceRecordSet(dns.RS_AAAA, matchWrrItem(1, "a23::4"), matchWrrPlaceholderItem(dns.RS_AAAA), matchWrrItem(1, "cef::1")),
				}),
			}),
		),
		Entry("prepares geolocation policy-routing change requests",
			[]*provider.ChangeRequest{
				{Action: provider.R_CREATE, Type: dns.RS_A, Addition: dnssetgeo1_0},
				{Action: provider.R_CREATE, Type: dns.RS_A, Addition: dnssetgeo1_2},
				{Action: provider.R_UPDATE, Type: dns.RS_CNAME, Addition: dnssetgeo2_2new, Deletion: dnssetgeo2_2old},
				{Action: provider.R_DELETE, Type: dns.RS_TXT, Deletion: dnssetgeo3_1},
			},
			geoStatus0,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w2.example.org.": matchGeoResourceRecordSet(dns.RS_CNAME, matchGeoItem("asia-east2", "some-other.example.org."), matchGeoItem("europe-west1", "some.example.org.")),
					"w3.example.org.": matchGeoResourceRecordSet(dns.RS_TXT, matchGeoItem("asia-east2", "\"bla\"", "\"foo\"")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchGeoResourceRecordSet(dns.RS_A, matchGeoItem("europe-west1", "1.1.2.0"), matchGeoItem("asia-east1", "1.1.2.2")),
					"w2.example.org.": matchGeoResourceRecordSet(dns.RS_CNAME, matchGeoItem("asia-east2", "some-other.example.org."), matchGeoItem("europe-west1", "some2.example.org.")),
				}),
			}),
		),
		Entry("prepares geolocation policy-routing change requests (merging)",
			[]*provider.ChangeRequest{
				{Action: provider.R_CREATE, Type: dns.RS_A, Addition: dnssetgeo1_0},
				{Action: provider.R_DELETE, Type: dns.RS_A, Deletion: dnssetgeo1_2},
			},
			geoStatus1,
			MatchFields(IgnoreExtras, Fields{
				"Deletions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchGeoResourceRecordSet(dns.RS_A, matchGeoItem("us-east1", "1.1.2.1"), matchGeoItem("asia-east1", "1.1.2.2")),
				}),
				"Additions": MatchAllElements(nameFunc, Elements{
					"w1.example.org.": matchGeoResourceRecordSet(dns.RS_A, matchGeoItem("europe-west1", "1.1.2.0"), matchGeoItem("us-east1", "1.1.2.1")),
				}),
			}),
		),
	)
})

func makeDNSSet(dnsName, typ string, ttl int64, targets ...string) *dns.DNSSet {
	set := dns.NewDNSSet(dns.DNSSetName{DNSName: dnsName}, nil)
	set.SetRecordSet(typ, ttl, targets...)
	return set
}

func makeDNSSetWrr(dnsName string, index, weight int, typ string, targets ...string) *dns.DNSSet {
	policy := &dns.RoutingPolicy{
		Type:       dns.RoutingPolicyWeighted,
		Parameters: map[string]string{"weight": fmt.Sprintf("%d", weight)},
	}
	set := dns.NewDNSSet(dns.DNSSetName{DNSName: dnsName, SetIdentifier: fmt.Sprintf("%d", index)}, policy)
	set.SetRecordSet(typ, 300, targets...)
	return set
}

func makeDNSSetWrrWrongWeight02(dnsName string, index int, typ string, targets ...string) *dns.DNSSet {
	set := makeDNSSetWrr(dnsName, index, 0, typ, targets...)
	set.RoutingPolicy.Parameters["weight"] = "0.2"
	return set
}

func makeDNSSetWrrMissingWeight(dnsName string, index int, typ string, targets ...string) *dns.DNSSet {
	set := makeDNSSetWrr(dnsName, index, 0, typ, targets...)
	delete(set.RoutingPolicy.Parameters, "weight")
	return set
}

func makeDNSSetGeo(dnsName string, location string, typ string, targets ...string) *dns.DNSSet {
	policy := &dns.RoutingPolicy{
		Type:       dns.RoutingPolicyGeoLocation,
		Parameters: map[string]string{"location": location},
	}
	set := dns.NewDNSSet(dns.DNSSetName{DNSName: dnsName, SetIdentifier: location}, policy)
	set.SetRecordSet(typ, 300, targets...)
	return set
}

func matchSimpleResourceRecordSet(typ string, ttl int64, targets ...string) types.GomegaMatcher {
	return PointTo(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(typ),
		"Ttl":     Equal(ttl),
		"Rrdatas": Equal(targets),
	}))
}

func itemNameFunc(index int, _ interface{}) string {
	return fmt.Sprintf("%d", index)
}

func geoItemNameFunc(element interface{}) string {
	if geo, ok := element.(*googledns.RRSetRoutingPolicyGeoPolicyGeoPolicyItem); ok {
		return geo.Location
	}
	return ""
}

func matchWrrResourceRecordSet(typ string, items ...types.GomegaMatcher) types.GomegaMatcher {
	elements := Elements{}
	for i, item := range items {
		elements[fmt.Sprintf("%d", i)] = item
	}

	return PointTo(MatchFields(IgnoreExtras, Fields{
		"Type": Equal(typ),
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

func matchWrrPlaceholderItem(typ string) types.GomegaMatcher {
	return PointTo(MatchFields(IgnoreExtras, Fields{
		"Weight":  Equal(float64(0)),
		"Rrdatas": Equal([]string{rrDefaultValue(typ)}),
	}))
}

func matchGeoResourceRecordSet(typ string, items ...geoItem) types.GomegaMatcher {
	elements := Elements{}
	for _, item := range items {
		elements[item.location] = PointTo(MatchFields(IgnoreExtras, Fields{
			"Location": Equal(item.location),
			"Rrdatas":  Equal(item.targets),
		}))
	}

	return PointTo(MatchFields(IgnoreExtras, Fields{
		"Type": Equal(typ),
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

func prepareSubmission(reqs []*provider.ChangeRequest, rrsetGetter rrsetGetterFunc) (*googledns.Change, error) {
	log := logger.NewContext("", "TestEnv")
	zone := provider.NewDNSHostedZone(TYPE_CODE, "test", "example.org", "", false)
	doneHandler := &testDoneHandler{}
	exec := NewExecution(log, nil, zone)
	for _, r := range reqs {
		r.Done = doneHandler
		exec.addChange(r)
	}
	if doneHandler.invalidCount > 0 {
		return nil, fmt.Errorf("errors: %d, last message: %s", doneHandler.invalidCount, doneHandler.lastMessage)
	}
	if err := exec.prepareSubmission(rrsetGetter); err != nil {
		return nil, err
	}
	return exec.change, nil
}

type testDoneHandler struct {
	invalidCount int
	failedCount  int
	lastMessage  string
}

var _ provider.DoneHandler = &testDoneHandler{}

func (h *testDoneHandler) SetInvalid(err error) {
	h.invalidCount++
	h.lastMessage = err.Error()
}

func (h *testDoneHandler) Failed(err error) {
	h.failedCount++
	h.lastMessage = err.Error()
}

func (h *testDoneHandler) Throttled() {}
func (h *testDoneHandler) Succeeded() {}
