// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

var _ = Describe("DNSSpecInput", func() {
	Describe("#GetDNSSpecInputForIngress", func() {
		var (
			log             = logr.Discard()
			gkv             = networkingv1.SchemeGroupVersion.WithKind("Ingress")
			annotationState = state.GetState().GetAnnotationState()
			ingress         *networkingv1.Ingress
		)

		BeforeEach(func() {
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{Host: "example.com"},
						{Host: "gardener.cloud"},
						{Host: "wikipedia.org"},
					},
				},
				Status: networkingv1.IngressStatus{
					LoadBalancer: networkingv1.IngressLoadBalancerStatus{},
				},
			}
		})

		It("should return nil when no dns names are specified", func() {
			input, err := common.GetDNSSpecInputForIngress(log, annotationState, gkv, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(input).To(BeNil())
		})

		It("should return an error when the dns names annotation is empty", func() {
			ingress.Annotations[dns.AnnotationDNSNames] = ""
			input, err := common.GetDNSSpecInputForIngress(log, annotationState, gkv, ingress)
			Expect(err).To(MatchError("empty value for annotation \"dns.gardener.cloud/dnsnames\""))
			Expect(input).To(BeNil())
		})

		It("should handle the wildcard dns name annotation", func() {
			ingress.Annotations[dns.AnnotationDNSNames] = "*"
			input, err := common.GetDNSSpecInputForIngress(log, annotationState, gkv, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(input.Names.ToSlice()).To(ConsistOf([]string{"example.com", "gardener.cloud", "wikipedia.org"}))
		})

		It("should handle a subset of dns names", func() {
			ingress.Annotations[dns.AnnotationDNSNames] = "example.com,gardener.cloud"
			input, err := common.GetDNSSpecInputForIngress(log, annotationState, gkv, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(input.Names.ToSlice()).To(ConsistOf([]string{"example.com", "gardener.cloud"}))
		})

		It("should reject dns names not found in the ingress rules", func() {
			ingress.Annotations[dns.AnnotationDNSNames] = "example.com,notfound.com"
			input, err := common.GetDNSSpecInputForIngress(log, annotationState, gkv, ingress)
			Expect(err).To(MatchError("annotated dns names notfound.com not declared by ingress"))
			Expect(input).To(BeNil())
		})

		It("should set ResolveTargetsToAddresses when annotation is present", func() {
			ingress.Annotations[dns.AnnotationDNSNames] = "example.com"
			ingress.Annotations[dns.AnnotationResolveTargetsToAddresses] = "true"
			input, err := common.GetDNSSpecInputForIngress(log, annotationState, gkv, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(input.ResolveTargetsToAddresses).To(Equal(ptr.To(true)))
		})

		It("should set IP Targets from ingress status", func() {
			ingress.Annotations[dns.AnnotationDNSNames] = "example.com"
			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{IP: "1.1.1.1"}}
			input, err := common.GetDNSSpecInputForIngress(log, annotationState, gkv, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(input.Targets.ToSlice()).To(ConsistOf([]string{"1.1.1.1"}))
		})

		It("should set hostname Targets from ingress status", func() {
			ingress.Annotations[dns.AnnotationDNSNames] = "example.com"
			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{Hostname: "https://example.org"}}
			input, err := common.GetDNSSpecInputForIngress(log, annotationState, gkv, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(input.Targets.ToSlice()).To(ConsistOf([]string{"https://example.org"}))
		})

		It("should prefer IP Targets over hostnames ingress status", func() {
			ingress.Annotations[dns.AnnotationDNSNames] = "example.com"
			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{IP: "1.1.1.1", Hostname: "https://example.org"}}
			input, err := common.GetDNSSpecInputForIngress(log, annotationState, gkv, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(input.Targets.ToSlice()).To(ConsistOf([]string{"1.1.1.1"}))
		})

		It("should collect multiple, unique Targets from ingress status", func() {
			ingress.Annotations[dns.AnnotationDNSNames] = "example.com"
			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{IP: "1.1.1.1"}, {IP: "1.0.0.1"}, {IP: "1.1.1.1"}}
			input, err := common.GetDNSSpecInputForIngress(log, annotationState, gkv, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(input.Targets.ToSlice()).To(ConsistOf([]string{"1.1.1.1", "1.0.0.1"}))
		})

		It("should set IPStack when annotation is present", func() {
			ingress.Annotations[dns.AnnotationDNSNames] = "example.com"
			ingress.Annotations[dns.AnnotationIPStack] = dns.AnnotationValueIPStackIPDualStack
			input, err := common.GetDNSSpecInputForIngress(log, annotationState, gkv, ingress)
			Expect(err).NotTo(HaveOccurred())
			Expect(input.IPStack).To(Equal("dual-stack"))
		})
	})

	Describe("#GetTargetsForService", func() {
		var svc *corev1.Service

		BeforeEach(func() {
			svc = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "svc1", Namespace: "ns"},
				Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
			}
		})

		It("should return empty targets for a non-LoadBalancer service", func() {
			svc.Spec.Type = corev1.ServiceTypeClusterIP
			targets := common.GetTargetsForService(svc, nil)
			Expect(targets.Len()).To(Equal(0))
		})

		It("should return empty targets when there are no load balancer ingresses", func() {
			targets := common.GetTargetsForService(svc, nil)
			Expect(targets.Len()).To(Equal(0))
		})

		It("should return IP targets", func() {
			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{IP: "1.2.3.4"},
				{IP: "5.6.7.8"},
			}
			targets := common.GetTargetsForService(svc, nil)
			Expect(targets.ToSlice()).To(ConsistOf("1.2.3.4", "5.6.7.8"))
		})

		It("should return hostname targets when IP is empty", func() {
			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{Hostname: "lb.example.com"},
			}
			targets := common.GetTargetsForService(svc, nil)
			Expect(targets.ToSlice()).To(ConsistOf("lb.example.com"))
		})

		It("should prefer IP over hostname when both are set", func() {
			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{IP: "1.2.3.4", Hostname: "lb.example.com"},
			}
			targets := common.GetTargetsForService(svc, nil)
			Expect(targets.ToSlice()).To(ConsistOf("1.2.3.4"))
		})

		It("should deduplicate IPs", func() {
			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{IP: "1.2.3.4"},
				{IP: "1.2.3.4"},
			}
			targets := common.GetTargetsForService(svc, nil)
			Expect(targets.ToSlice()).To(ConsistOf("1.2.3.4"))
		})

		It("should use openstack load-balancer-address annotation when hostname is set", func() {
			svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{Hostname: "proxy.example.com"},
			}
			annotations := map[string]string{
				dns.AnnotationOpenStackLoadBalancerAddress: "10.0.0.1",
			}
			targets := common.GetTargetsForService(svc, annotations)
			Expect(targets.ToSlice()).To(ConsistOf("10.0.0.1"))
		})
	})

	Describe("#GetTargetsForIngress", func() {
		var ingress *networkingv1.Ingress

		BeforeEach(func() {
			ingress = &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "ing1", Namespace: "ns"},
			}
		})

		It("should return empty targets when there are no load balancer ingresses", func() {
			targets := common.GetTargetsForIngress(ingress)
			Expect(targets.Len()).To(Equal(0))
		})

		It("should return IP targets", func() {
			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{
				{IP: "1.2.3.4"},
				{IP: "5.6.7.8"},
			}
			targets := common.GetTargetsForIngress(ingress)
			Expect(targets.ToSlice()).To(ConsistOf("1.2.3.4", "5.6.7.8"))
		})

		It("should return hostname targets when no IPs are present", func() {
			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{
				{Hostname: "lb.example.com"},
			}
			targets := common.GetTargetsForIngress(ingress)
			Expect(targets.ToSlice()).To(ConsistOf("lb.example.com"))
		})

		It("should prefer IPs over hostnames", func() {
			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{
				{IP: "1.2.3.4", Hostname: "lb.example.com"},
			}
			targets := common.GetTargetsForIngress(ingress)
			Expect(targets.ToSlice()).To(ConsistOf("1.2.3.4"))
		})

		It("should deduplicate IPs", func() {
			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{
				{IP: "1.2.3.4"},
				{IP: "1.2.3.4"},
			}
			targets := common.GetTargetsForIngress(ingress)
			Expect(targets.ToSlice()).To(ConsistOf("1.2.3.4"))
		})

		It("should deduplicate hostnames", func() {
			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{
				{Hostname: "lb.example.com"},
				{Hostname: "lb.example.com"},
			}
			targets := common.GetTargetsForIngress(ingress)
			Expect(targets.ToSlice()).To(ConsistOf("lb.example.com"))
		})
	})

	Describe("#MatchesWildcardSingleSubdomain", func() {
		It("should match when host is a single-level subdomain of wildcard h", func() {
			Expect(common.MatchesWildcardSingleSubdomain("foo.gardener.cloud", "*.gardener.cloud")).To(BeTrue())
		})

		It("should not match when h is not a wildcard", func() {
			Expect(common.MatchesWildcardSingleSubdomain("docs.gardener.cloud", "docs.gardener.cloud")).To(BeFalse())
		})

		It("should not match when host is the base domain of wildcard h", func() {
			Expect(common.MatchesWildcardSingleSubdomain("gardener.cloud", "*.gardener.cloud")).To(BeFalse())
		})

		It("should not match when host has multiple levels below wildcard h", func() {
			Expect(common.MatchesWildcardSingleSubdomain("a.b.gardener.cloud", "*.gardener.cloud")).To(BeFalse())
		})

		It("should not match an unrelated domain", func() {
			Expect(common.MatchesWildcardSingleSubdomain("example.com", "*.gardener.cloud")).To(BeFalse())
		})

		It("should match with a deeper base domain", func() {
			Expect(common.MatchesWildcardSingleSubdomain("foo.api.gardener.cloud", "*.api.gardener.cloud")).To(BeTrue())
		})
	})
})
