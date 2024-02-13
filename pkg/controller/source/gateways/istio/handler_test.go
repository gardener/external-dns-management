/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package istio

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	networkingv1beta1 "istio.io/api/networking/v1beta1"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/external-dns-management/pkg/dns"
	dnssource "github.com/gardener/external-dns-management/pkg/dns/source"
)

var _ = Describe("Istio Gateway Handler", func() {
	var (
		service1 = &corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{
				{IP: "1.2.3.4"},
			},
		}
		service2 = &corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{
				{Hostname: "lb-example.com"},
			},
		}
		ingress1 = &corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{
				{Hostname: "ingress-lb.example.com"},
			},
		}
		defaultSources = map[string]*corev1.LoadBalancerStatus{
			"app=istio-ingressgateway,name=service1": service1,
			"app=istio-ingressgateway,name=service2": service2,
			"ingress=foo/bar":                        ingress1,
		}
		selectorService1 = map[string]string{"app": "istio-ingressgateway", "name": "service1"}
		selectorService2 = map[string]string{"app": "istio-ingressgateway", "name": "service2"}
		log              = logger.NewContext("", "TestEnv")
		emptyDNSInfo     = &dnssource.DNSInfo{Names: dns.DNSNameSet{}}

		vsvc1 = &istionetworkingv1beta1.VirtualService{
			Spec: networkingv1beta1.VirtualService{
				Gateways: []string{"test/g1"},
				Hosts:    []string{"foo.example.com", "bar.example.com"},
			},
		}
		vsvc2 = &istionetworkingv1beta1.VirtualService{
			Spec: networkingv1beta1.VirtualService{
				Gateways: []string{"test/g1"},
				Hosts:    []string{"foo.example.com"},
			},
		}
		vsvc3 = &istionetworkingv1beta1.VirtualService{
			Spec: networkingv1beta1.VirtualService{
				Gateways: []string{"test/g2"},
				Hosts:    []string{"bla.example.com"},
			},
		}
		virtualServices = []*istionetworkingv1beta1.VirtualService{vsvc1, vsvc2, vsvc3}
	)

	var _ = DescribeTable("GetDNSInfo",
		func(sources map[string]*corev1.LoadBalancerStatus, gateway *istionetworkingv1beta1.Gateway, expectedInfo *dnssource.DNSInfo) {
			lister := &testResourceLister{sources: sources, virtualServices: virtualServices}
			state := newState()
			handler, err := newGatewaySourceWithResourceLister(lister, state)
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
		Entry("unmanaged gateway", defaultSources, &istionetworkingv1beta1.Gateway{
			Spec: networkingv1beta1.Gateway{
				Servers: []*networkingv1beta1.Server{
					{Hosts: []string{"a.example.com"}},
				},
				Selector: selectorService1,
			},
		}, emptyDNSInfo),
		Entry("assigned gateway to service with IP", defaultSources, &istionetworkingv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "*"},
			},
			Spec: networkingv1beta1.Gateway{
				Servers: []*networkingv1beta1.Server{
					{Hosts: []string{"a.example.com"}},
				},
				Selector: selectorService1,
			},
		}, makeDNSInfo([]string{"a.example.com"}, []string{"1.2.3.4"})),
		Entry("assigned gateway to service with annotated targets", defaultSources, &istionetworkingv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					dnssource.DNS_ANNOTATION: "*",
					TargetsAnnotation:        "10.0.0.10,10.0.0.11",
				},
			},
			Spec: networkingv1beta1.Gateway{
				Servers: []*networkingv1beta1.Server{
					{Hosts: []string{"a.example.com"}},
				},
				Selector: selectorService1,
			},
		}, makeDNSInfo([]string{"a.example.com"}, []string{"10.0.0.10", "10.0.0.11"})),
		Entry("assigned gateway to service with annotated ingress", defaultSources, &istionetworkingv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					dnssource.DNS_ANNOTATION:      "*",
					IngressTargetSourceAnnotation: "foo/bar",
				},
			},
			Spec: networkingv1beta1.Gateway{
				Servers: []*networkingv1beta1.Server{
					{Hosts: []string{"a.example.com"}},
				},
				Selector: selectorService1,
			},
		}, makeDNSInfo([]string{"a.example.com"}, []string{"ingress-lb.example.com"})),
		Entry("assigned gateway to service with hostname", defaultSources, &istionetworkingv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "*"},
			},
			Spec: networkingv1beta1.Gateway{
				Servers: []*networkingv1beta1.Server{
					{Hosts: []string{"ns1/b.example.com"}},
				},
				Selector: selectorService2,
			},
		}, makeDNSInfo([]string{"b.example.com"}, []string{"lb-example.com"})),
		Entry("ignore '*' hosts", defaultSources, &istionetworkingv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "all"},
			},
			Spec: networkingv1beta1.Gateway{
				Servers: []*networkingv1beta1.Server{
					{Hosts: []string{"*", "ns2/c.example2.com"}},
				},
				Selector: selectorService2,
			},
		}, makeDNSInfo([]string{"c.example2.com"}, []string{"lb-example.com"})),
		Entry("selective hosts", defaultSources, &istionetworkingv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "a.example.com,c.example.com"},
			},
			Spec: networkingv1beta1.Gateway{
				Servers: []*networkingv1beta1.Server{
					{Hosts: []string{"*/a.example.com", "ns2/c.example.com", "d.example.com"}},
				},
				Selector: selectorService2,
			},
		}, makeDNSInfo([]string{"a.example.com", "c.example.com"}, []string{"lb-example.com"})),
		Entry("unmatched host in DNS annotation", defaultSources, &istionetworkingv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "a.example.com,c.example.com"},
			},
			Spec: networkingv1beta1.Gateway{
				Servers: []*networkingv1beta1.Server{
					{Hosts: []string{"*/a.example.com"}},
				},
				Selector: selectorService2,
			},
		}, nil),
		Entry("gateway with virtual services", defaultSources, &istionetworkingv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "test",
				Name:        "g1",
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "*"},
			},
			Spec: networkingv1beta1.Gateway{
				Servers: []*networkingv1beta1.Server{
					{Hosts: []string{"*.example.com"}},
				},
				Selector: selectorService2,
			},
		}, makeDNSInfo([]string{"*.example.com"}, []string{"lb-example.com"})),
		Entry("gateway with wildcard host and virtual services", defaultSources, &istionetworkingv1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "test",
				Name:        "g1",
				Annotations: map[string]string{dnssource.DNS_ANNOTATION: "*"},
			},
			Spec: networkingv1beta1.Gateway{
				Servers: []*networkingv1beta1.Server{
					{Hosts: []string{"*"}},
				},
				Selector: selectorService2,
			},
		}, makeDNSInfo([]string{"foo.example.com", "bar.example.com"}, []string{"lb-example.com"})),
	)
})

type testResourceLister struct {
	sources         map[string]*corev1.LoadBalancerStatus
	virtualServices []*istionetworkingv1beta1.VirtualService
}

var _ resourceLister = &testResourceLister{}

func (t *testResourceLister) ListServices(selectors map[string]string) ([]resources.ObjectData, error) {
	ls, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: selectors})
	Expect(err).To(Succeed())

	lbStatus := t.sources[ls.String()]
	if lbStatus == nil {
		return nil, nil
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
			Name:      "svc",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: *lbStatus,
			Conditions:   nil,
		},
	}

	return []resources.ObjectData{svc}, nil
}

func (t *testResourceLister) GetIngress(name resources.ObjectName) (resources.ObjectData, error) {
	lbStatus := t.sources["ingress="+name.String()]
	if lbStatus == nil {
		return nil, fmt.Errorf("not found")
	}

	ilbstatus := networkingv1.IngressLoadBalancerStatus{}
	for _, item := range lbStatus.Ingress {
		ilbstatus.Ingress = append(ilbstatus.Ingress, networkingv1.IngressLoadBalancerIngress{
			IP:       item.IP,
			Hostname: item.Hostname,
		})
	}
	ingress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: name.Namespace(),
			Name:      name.Name(),
		},
		Status: networkingv1.IngressStatus{
			LoadBalancer: ilbstatus,
		},
	}
	return &ingress, nil
}

func (t *testResourceLister) ListVirtualServices(gateway *resources.ObjectName) ([]resources.ObjectData, error) {
	var list []resources.ObjectData
	name := ""
	if gateway != nil {
		name = (*gateway).Namespace() + "/" + (*gateway).Name()
	}
outer:
	for _, vsvc := range t.virtualServices {
		for _, gw := range vsvc.Spec.Gateways {
			if name == "" || gw == name {
				list = append(list, vsvc)
				continue outer
			}
		}
	}
	return list, nil
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
