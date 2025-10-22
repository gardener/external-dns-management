// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/service"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("Add", func() {
	Describe("#RelevantServicePredicate", func() {
		var (
			servicePredicate predicate.Predicate
			svc              *corev1.Service
			svcNew           *corev1.Service

			test func(*corev1.Service, *corev1.Service, types.GomegaMatcher, types.GomegaMatcher)
		)

		BeforeEach(func() {
			servicePredicate = RelevantServicePredicate(dns.DefaultClass)

			svc = &corev1.Service{}
			svcNew = &corev1.Service{}

			test = func(
				svc *corev1.Service,
				svcNew *corev1.Service,
				match types.GomegaMatcher,
				matchUpdate types.GomegaMatcher,
			) {
				Expect(servicePredicate.Create(event.CreateEvent{Object: svc})).To(match)
				Expect(servicePredicate.Update(event.UpdateEvent{ObjectOld: svc, ObjectNew: svcNew})).To(matchUpdate)
				Expect(servicePredicate.Delete(event.DeleteEvent{Object: svc})).To(match)
				Expect(servicePredicate.Generic(event.GenericEvent{Object: svc})).To(BeFalse())
			}
		})

		It("should handle nil objects as expected", func() {
			test(nil, nil, BeFalse(), BeFalse())
		})

		It("should handle empty objects as expected", func() {
			test(svc, svcNew, BeFalse(), BeFalse())
		})

		It("should handle services of type LoadBalancer and secret name annotation as expected", func() {
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			svc.Annotations = map[string]string{dns.AnnotationDNSNames: "foo.example.com"}
			svcNew.Spec.Type = corev1.ServiceTypeLoadBalancer
			svcNew.Annotations = map[string]string{dns.AnnotationDNSNames: "foo.example.com"}
			test(svc, svcNew, BeTrue(), BeTrue())
		})

		It("should handle services without secretname annotation as expected", func() {
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			svcNew.Spec.Type = corev1.ServiceTypeLoadBalancer
			test(svc, svcNew, BeFalse(), BeFalse())
		})

		It("should handle services of irrelevant type as expected", func() {
			svc.Spec.Type = corev1.ServiceTypeClusterIP
			svc.Annotations = map[string]string{dns.AnnotationDNSNames: "foo.example.com"}
			svcNew.Spec.Type = corev1.ServiceTypeClusterIP
			svcNew.Annotations = map[string]string{dns.AnnotationDNSNames: "foo.example.com"}
			test(svc, svcNew, BeFalse(), BeFalse())
		})

		It("should handle services of wrong class as expected", func() {
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			svc.Annotations = map[string]string{dns.AnnotationDNSNames: "foo.example.com"}
			svc.Annotations[dns.AnnotationClass] = "bar"
			svcNew.Spec.Type = corev1.ServiceTypeLoadBalancer
			svcNew.Annotations = map[string]string{dns.AnnotationDNSNames: "foo.example.com"}
			svcNew.Annotations[dns.AnnotationClass] = dns.DefaultClass
			test(svc, svcNew, BeFalse(), BeTrue())
		})
	})

	Describe("#MapDNSEntryToService", func() {
		var (
			ctx   context.Context
			entry *dnsv1alpha1.DNSEntry
		)

		BeforeEach(func() {
			ctx = context.Background()
			entry = &dnsv1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-entry",
					Namespace: "test-namespace",
				},
			}
		})

		It("should return nil for non-DNSEntry objects", func() {
			svc := &corev1.Service{}
			result := MapDNSEntryToService(ctx, svc)
			Expect(result).To(BeNil())
		})

		It("should return request for Service owner reference", func() {
			entry.OwnerReferences = []metav1.OwnerReference{
				{
					Kind:       "Service",
					APIVersion: "v1",
					Name:       "test-service",
				},
			}
			result := MapDNSEntryToService(ctx, entry)
			Expect(result).To(HaveLen(1))
			Expect(result[0].NamespacedName.Namespace).To(Equal("test-namespace"))
			Expect(result[0].NamespacedName.Name).To(Equal("test-service"))
		})

		It("should return nil for non-Service owner reference", func() {
			entry.OwnerReferences = []metav1.OwnerReference{
				{
					Kind:       "Pod",
					APIVersion: "v1",
					Name:       "test-pod",
				},
			}
			result := MapDNSEntryToService(ctx, entry)
			Expect(result).To(BeNil())
		})

		It("should return nil for wrong API version", func() {
			entry.OwnerReferences = []metav1.OwnerReference{
				{
					Kind:       "Service",
					APIVersion: "v2",
					Name:       "test-service",
				},
			}
			result := MapDNSEntryToService(ctx, entry)
			Expect(result).To(BeNil())
		})

		It("should handle annotated owners with valid Service prefix", func() {
			entry.Annotations = map[string]string{
				"resources.gardener.cloud/owners": "cluster1:/Service/test-namespace/test-service",
			}
			result := MapDNSEntryToService(ctx, entry)
			Expect(result).To(HaveLen(1))
			Expect(result[0].NamespacedName.Namespace).To(Equal("test-namespace"))
			Expect(result[0].NamespacedName.Name).To(Equal("test-service"))
		})

		It("should handle multiple annotated owners", func() {
			entry.Annotations = map[string]string{
				"resources.gardener.cloud/owners": "cluster1:/Service/ns1/svc1,cluster2:/Service/ns2/svc2",
			}
			result := MapDNSEntryToService(ctx, entry)
			Expect(result).To(HaveLen(2))
			Expect(result[0].NamespacedName.Namespace).To(Equal("ns1"))
			Expect(result[0].NamespacedName.Name).To(Equal("svc1"))
			Expect(result[1].NamespacedName.Namespace).To(Equal("ns2"))
			Expect(result[1].NamespacedName.Name).To(Equal("svc2"))
		})

		It("should ignore annotated owners without Service prefix", func() {
			entry.Annotations = map[string]string{
				"resources.gardener.cloud/owners": "cluster1:/Pod/test-namespace/test-pod",
			}
			result := MapDNSEntryToService(ctx, entry)
			Expect(result).To(BeEmpty())
		})
	})
})
