// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/service"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("Add", func() {
	Describe("#IsRelevantSourceObject", func() {
		var (
			svc *corev1.Service

			actuator   = &Actuator{}
			reconciler = common.NewSourceReconciler(actuator)
			test       = func(svc *corev1.Service, match types.GomegaMatcher) {
				Expect(actuator.IsRelevantSourceObject(reconciler, svc)).To(match)
			}
		)

		BeforeEach(func() {
			svc = &corev1.Service{}
		})

		It("should handle nil objects as expected", func() {
			test(nil, BeFalse())
		})

		It("should handle empty objects as expected", func() {
			test(svc, BeFalse())
		})

		It("should handle services of type LoadBalancer and secret name annotation as expected", func() {
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			svc.Annotations = map[string]string{dns.AnnotationDNSNames: "foo.example.com"}
			test(svc, BeTrue())
		})

		It("should handle services without secretname annotation as expected", func() {
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			test(svc, BeFalse())
		})

		It("should handle services of irrelevant type as expected", func() {
			svc.Spec.Type = corev1.ServiceTypeClusterIP
			svc.Annotations = map[string]string{dns.AnnotationDNSNames: "foo.example.com"}
			test(svc, BeFalse())
		})

		It("should handle services of wrong class as expected", func() {
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			svc.Annotations = map[string]string{dns.AnnotationDNSNames: "foo.example.com"}
			svc.Annotations[dns.AnnotationClass] = "bar"
			test(svc, BeFalse())
			svc.Annotations[dns.AnnotationClass] = dns.DefaultClass
			test(svc, BeTrue())
		})
	})
})
