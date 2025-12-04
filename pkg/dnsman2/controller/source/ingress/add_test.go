// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ingress_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/event"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/ingress"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

var _ = Describe("Add", func() {
	Describe("#RelevantIngressPredicate", func() {
		var (
			ing, ingNew *networkingv1.Ingress
		)

		reconciler := &ingress.Reconciler{
			ReconcilerBase: common.ReconcilerBase{
				GVK:   schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
				State: state.GetState().GetAnnotationState(),
			},
		}
		predicate := reconciler.RelevantIngressPredicate()
		test := func(ing, ingNew *networkingv1.Ingress, match, matchUpdate types.GomegaMatcher) {
			ExpectWithOffset(1, predicate.Create(event.CreateEvent{Object: ing})).To(match)
			ExpectWithOffset(1, predicate.Update(event.UpdateEvent{ObjectOld: ing, ObjectNew: ingNew})).To(matchUpdate)
			ExpectWithOffset(1, predicate.Delete(event.DeleteEvent{Object: ing})).To(match)
			ExpectWithOffset(1, predicate.Generic(event.GenericEvent{Object: ing})).To(BeFalse())
		}

		BeforeEach(func() {
			ing = &networkingv1.Ingress{}
			ingNew = &networkingv1.Ingress{}
		})

		It("should handle nil objects as expected", func() {
			test(nil, nil, BeFalse(), BeFalse())
		})

		It("should handle empty objects as expected", func() {
			test(ing, ingNew, BeFalse(), BeFalse())
		})

		It("should handle an ingress with annotations as expected", func() {
			ing.Annotations = map[string]string{
				"dns.gardener.cloud/class":    "gardendns",
				"dns.gardener.cloud/dnsnames": "example.com",
			}
			ingNew.Annotations = map[string]string{
				"dns.gardener.cloud/class":    "gardendns",
				"dns.gardener.cloud/dnsnames": "example.com",
			}
			test(ing, ingNew, BeTrue(), BeTrue())
		})

		It("should handle an ingress with missing DNS names annotation as expected", func() {
			ing.Annotations = map[string]string{
				"dns.gardener.cloud/class": "gardendns",
			}
			ingNew.Annotations = map[string]string{
				"dns.gardener.cloud/class": "gardendns",
			}
			test(ing, ingNew, BeFalse(), BeFalse())
		})

		It("should handle an ingress with a wrong DNS class annotation as expected", func() {
			ing.Annotations = map[string]string{
				"dns.gardener.cloud/class": "jardindns",
			}
			ingNew.Annotations = map[string]string{
				"dns.gardener.cloud/class": "jardindns",
			}
			test(ing, ingNew, BeFalse(), BeFalse())
		})
	})

	Describe("#MapDNSAnnotationToIngress", func() {
		var (
			ctx        context.Context
			annotation *dnsv1alpha1.DNSAnnotation
		)

		BeforeEach(func() {
			ctx = context.Background()
			annotation = &dnsv1alpha1.DNSAnnotation{}
		})

		It("should return nil for non-DNSAnnotation objects", func() {
			Expect(ingress.MapDNSAnnotationToIngress(ctx, &networkingv1.Ingress{})).To(BeNil())
		})

		It("should return nil for DNSAnnotation objects referencing non-Ingress resources", func() {
			annotation.Spec.ResourceRef.Kind = "Pod"
			Expect(ingress.MapDNSAnnotationToIngress(ctx, annotation)).To(BeNil())
		})

		It("should return nil for DNSAnnotation objects referencing non-networking API version", func() {
			annotation.Spec.ResourceRef.APIVersion = "v1"
			Expect(ingress.MapDNSAnnotationToIngress(ctx, annotation)).To(BeNil())
		})

		It("should return a reconcile request for a DNSAnnotation referencing an Ingress", func() {
			annotation.Spec.ResourceRef.Kind = "Ingress"
			annotation.Spec.ResourceRef.APIVersion = "networking.k8s.io/v1"
			annotation.Spec.ResourceRef.Namespace = "kube-system"
			annotation.Spec.ResourceRef.Name = "my-ingress"

			requests := ingress.MapDNSAnnotationToIngress(ctx, annotation)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].NamespacedName.Namespace).To(Equal("kube-system"))
			Expect(requests[0].NamespacedName.Name).To(Equal("my-ingress"))
		})
	})
})
