// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gatewayapi

import (
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/gardener/external-dns-management/pkg/dns"
	dnssource "github.com/gardener/external-dns-management/pkg/dns/source"
)

var _ = Describe("Kubernetes Networking Gateway Handler", func() {
	var (
		route1 = &gatewayapisv1.HTTPRoute{
			Spec: gatewayapisv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayapisv1.CommonRouteSpec{ParentRefs: []gatewayapisv1.ParentReference{
					{
						Namespace: ptr.To(gatewayapisv1.Namespace("test")),
						Name:      "g1",
					},
				}},
				Hostnames: []gatewayapisv1.Hostname{"foo.example.com", "bar.example.com"},
			},
		}
		route2 = &gatewayapisv1.HTTPRoute{
			Spec: gatewayapisv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayapisv1.CommonRouteSpec{ParentRefs: []gatewayapisv1.ParentReference{
					{
						Namespace: ptr.To(gatewayapisv1.Namespace("test")),
						Name:      "g1",
					},
				}},
				Hostnames: []gatewayapisv1.Hostname{"foo.example.com"},
			},
		}
		route3 = &gatewayapisv1.HTTPRoute{
			Spec: gatewayapisv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayapisv1.CommonRouteSpec{ParentRefs: []gatewayapisv1.ParentReference{
					{
						Namespace: ptr.To(gatewayapisv1.Namespace("test")),
						Name:      "g2",
					},
				}},
				Hostnames: []gatewayapisv1.Hostname{"bla.example.com"},
			},
		}
		routes = []*gatewayapisv1.HTTPRoute{route1, route2, route3}

		log          = logger.NewContext("", "TestEnv")
		emptyDNSInfo = &dnssource.DNSInfo{Names: dns.DNSNameSet{}}
	)

	var _ = DescribeTable("GetDNSInfo",
		func(gateway *gatewayapisv1.Gateway, httpRoutes []*gatewayapisv1.HTTPRoute, expectedInfo *dnssource.DNSInfo) {
			handler, err := newGatewaySourceWithRouteLister(&testRouteLister{routes: httpRoutes}, newState())
			Expect(err).To(Succeed())
			current := &dnssource.DNSCurrentState{Names: map[dns.DNSSetName]*dnssource.DNSState{}, Targets: utils.StringSet{}}
			annos := gateway.GetAnnotations()
			current.AnnotatedNames = utils.StringSet{}
			current.AnnotatedNames.AddAllSplittedSelected(annos[dnssource.DNS_ANNOTATION], utils.StandardNonEmptyStringElement)

			actual, err := handler.GetDNSInfo(log, gateway, current)
			if err != nil {
				if expectedInfo != nil {
					Fail("unexpected error: " + err.Error())
				}
				return
			}
			if expectedInfo == nil {
				Fail("expected error, but got DNSInfo")
				return
			}
			Expect(*actual).To(Equal(*expectedInfo))
		},
		Entry("should be empty if there are no listeners", &gatewayapisv1.Gateway{
			Spec: gatewayapisv1.GatewaySpec{},
		}, nil, emptyDNSInfo),
		Entry("should have empty targets if there are no addresses", &gatewayapisv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "test",
				Name:        "g1",
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "*"},
			},
			Spec: gatewayapisv1.GatewaySpec{
				Listeners: []gatewayapisv1.Listener{
					{Hostname: ptr.To(gatewayapisv1.Hostname("a.example.com"))},
				},
			},
		}, nil, makeDNSInfo([]string{"a.example.com"}, nil)),
		Entry("assigned gateway to service with IP", &gatewayapisv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "test",
				Name:        "g1",
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "*"},
			},
			Spec: gatewayapisv1.GatewaySpec{
				Listeners: []gatewayapisv1.Listener{
					{Hostname: ptr.To(gatewayapisv1.Hostname("a.example.com"))},
					{Hostname: ptr.To(gatewayapisv1.Hostname("b.example.com"))},
				},
			},
			Status: gatewayapisv1.GatewayStatus{
				Addresses: []gatewayapisv1.GatewayStatusAddress{
					{
						Type:  ptr.To(gatewayapisv1.NamedAddressType),
						Value: "foo",
					},
					{
						Type:  ptr.To(gatewayapisv1.IPAddressType),
						Value: "1.2.3.4",
					},
				},
			},
		}, nil, makeDNSInfo([]string{"a.example.com", "b.example.com"}, []string{"1.2.3.4"})),
		Entry("assigned gateway to service with IP (no address type)", &gatewayapisv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "test",
				Name:        "g1",
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "*"},
			},
			Spec: gatewayapisv1.GatewaySpec{
				Listeners: []gatewayapisv1.Listener{
					{Hostname: ptr.To(gatewayapisv1.Hostname("a.example.com"))},
					{Hostname: ptr.To(gatewayapisv1.Hostname("b.example.com"))},
				},
			},
			Status: gatewayapisv1.GatewayStatus{
				Addresses: []gatewayapisv1.GatewayStatusAddress{
					{
						Value: "1.2.3.4",
					},
					{
						Value: "5.6.7.8",
					},
				},
			},
		}, nil, makeDNSInfo([]string{"a.example.com", "b.example.com"}, []string{"1.2.3.4", "5.6.7.8"})),
		Entry("assigned gateway to service with hostname", &gatewayapisv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "test",
				Name:        "g1",
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "*"},
			},
			Spec: gatewayapisv1.GatewaySpec{
				Listeners: []gatewayapisv1.Listener{
					{Hostname: ptr.To(gatewayapisv1.Hostname("b.example.com"))},
				},
			},
			Status: gatewayapisv1.GatewayStatus{
				Addresses: []gatewayapisv1.GatewayStatusAddress{
					{
						Type:  ptr.To(gatewayapisv1.NamedAddressType),
						Value: "foo",
					},
					{
						Type:  ptr.To(gatewayapisv1.IPAddressType),
						Value: "lb-example.com",
					},
				},
			},
		}, nil, makeDNSInfo([]string{"b.example.com"}, []string{"lb-example.com"})),
		Entry("assigned gateway to service with wildcard hostname and HTTPRoutes", &gatewayapisv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "test",
				Name:        "g1",
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "*"},
			},
			Spec: gatewayapisv1.GatewaySpec{
				Listeners: []gatewayapisv1.Listener{
					{Hostname: ptr.To(gatewayapisv1.Hostname("*.example.com"))},
				},
			},
			Status: gatewayapisv1.GatewayStatus{
				Addresses: []gatewayapisv1.GatewayStatusAddress{
					{
						Type:  ptr.To(gatewayapisv1.NamedAddressType),
						Value: "foo",
					},
					{
						Type:  ptr.To(gatewayapisv1.IPAddressType),
						Value: "lb-example.com",
					},
				},
			},
		}, routes, makeDNSInfo([]string{"*.example.com"}, []string{"lb-example.com"})),
		Entry("assigned gateway to service with hostname and HTTPRoutes", &gatewayapisv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "test",
				Name:        "g1",
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "*"},
			},
			Spec: gatewayapisv1.GatewaySpec{
				Listeners: []gatewayapisv1.Listener{
					{Hostname: ptr.To(gatewayapisv1.Hostname("b.example.com"))},
				},
			},
			Status: gatewayapisv1.GatewayStatus{
				Addresses: []gatewayapisv1.GatewayStatusAddress{
					{
						Type:  ptr.To(gatewayapisv1.NamedAddressType),
						Value: "foo",
					},
					{
						Type:  ptr.To(gatewayapisv1.IPAddressType),
						Value: "lb-example.com",
					},
				},
			},
		}, routes, makeDNSInfo([]string{"b.example.com", "bar.example.com", "foo.example.com"}, []string{"lb-example.com"})),
		Entry("unmatched host in DNS annotation", &gatewayapisv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "test",
				Name:        "g1",
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "a.example.com,c.example.com"},
			},
			Spec: gatewayapisv1.GatewaySpec{
				Listeners: []gatewayapisv1.Listener{
					{Hostname: ptr.To(gatewayapisv1.Hostname("a.example.com"))},
					{Hostname: ptr.To(gatewayapisv1.Hostname("b.example.com"))},
				},
			},
			Status: gatewayapisv1.GatewayStatus{
				Addresses: []gatewayapisv1.GatewayStatusAddress{
					{
						Type:  ptr.To(gatewayapisv1.NamedAddressType),
						Value: "foo",
					},
					{
						Type:  ptr.To(gatewayapisv1.IPAddressType),
						Value: "1.2.3.4",
					},
				},
			},
		}, nil, nil),
	)
})

type testRouteLister struct {
	routes []*gatewayapisv1.HTTPRoute
}

var _ httpRouteLister = &testRouteLister{}

func (t testRouteLister) ListHTTPRoutes(gateway *resources.ObjectName) ([]resources.ObjectData, error) {
	var filtered []resources.ObjectData
	for _, r := range t.routes {
		for _, ref := range r.Spec.ParentRefs {
			if gateway == nil ||
				(ref.Namespace == nil || string(*ref.Namespace) == (*gateway).Namespace()) && string(ref.Name) == (*gateway).Name() {
				filtered = append(filtered, r)
			}
		}
	}
	return filtered, nil
}

func makeDNSInfo(names, targets []string) *dnssource.DNSInfo {
	nameSet := dns.DNSNameSet{}
	for _, name := range names {
		nameSet.Add(dns.DNSSetName{DNSName: name})
	}
	var targetSet utils.StringSet
	if targets != nil {
		targetSet = utils.NewStringSet(targets...)
	}
	return &dnssource.DNSInfo{Names: nameSet, Targets: targetSet}
}
