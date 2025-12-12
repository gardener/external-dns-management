// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
})
