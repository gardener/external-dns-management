// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/service"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("Add", func() {
	Describe("#Predicate", func() {
		var (
			servicePredicate predicate.Predicate
			svc              *corev1.Service
			svcNew           *corev1.Service

			test func(*corev1.Service, *corev1.Service, types.GomegaMatcher, types.GomegaMatcher)
		)

		BeforeEach(func() {
			servicePredicate = Predicate(dns.DefaultClass)

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
})
