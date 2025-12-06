// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
)

var _ = Describe("Add", func() {
	Describe("#RelevantServicePredicate", func() {
		var (
			cmPredicate predicate.Predicate
			cm          *corev1.ConfigMap
			cmNew       *corev1.ConfigMap

			test func(*corev1.ConfigMap, *corev1.ConfigMap, types.GomegaMatcher, types.GomegaMatcher)
		)

		BeforeEach(func() {
			cmPredicate = common.RelevantSourceObjectPredicate(nil, func(_ *common.SourceReconciler[*corev1.ConfigMap], cm *corev1.ConfigMap) bool {
				return cm != nil && cm.Data["Relevant"] == "true"
			})

			cm = &corev1.ConfigMap{}
			cmNew = &corev1.ConfigMap{}

			test = func(
				cm *corev1.ConfigMap,
				cmNew *corev1.ConfigMap,
				match types.GomegaMatcher,
				matchUpdate types.GomegaMatcher,
			) {
				Expect(cmPredicate.Create(event.CreateEvent{Object: cm})).To(match)
				Expect(cmPredicate.Update(event.UpdateEvent{ObjectOld: cm, ObjectNew: cmNew})).To(matchUpdate)
				Expect(cmPredicate.Delete(event.DeleteEvent{Object: cm})).To(match)
				Expect(cmPredicate.Generic(event.GenericEvent{Object: cm})).To(BeFalse())
			}
		})

		It("should handle nil objects as expected", func() {
			test(nil, nil, BeFalse(), BeFalse())
		})

		It("should handle empty objects as expected", func() {
			test(cm, cmNew, BeFalse(), BeFalse())
		})

		It("should handle relevant source objects as expected", func() {
			cm.Data = map[string]string{"Relevant": "true"}
			cmNew.Data = map[string]string{"Relevant": "true"}
			test(cm, cmNew, BeTrue(), BeTrue())
		})

		It("should handle services of wrong class as expected", func() {
			cm.Data = nil
			cmNew.Data = map[string]string{"Relevant": "true"}
			test(cm, cmNew, BeFalse(), BeTrue())
		})
	})

	Describe("#MapDNSAnnotationToSourceRequest", func() {
		var (
			ctx        context.Context
			annotation *dnsv1alpha1.DNSAnnotation
			mapper     func(context.Context, client.Object) []reconcile.Request
		)

		BeforeEach(func() {
			ctx = context.Background()
			annotation = &dnsv1alpha1.DNSAnnotation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-annotation",
					Namespace: "test-namespace",
				},
				Spec: dnsv1alpha1.DNSAnnotationSpec{
					ResourceRef: dnsv1alpha1.ResourceReference{
						Kind:       "ConfigMap",
						APIVersion: "v1",
						Name:       "test-object",
						Namespace:  "test-namespace",
					},
				},
			}
			mapper = common.MapDNSAnnotationToSourceRequest(schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "ConfigMap",
			})
		})

		It("should return nil for non-DNSAnnotation objects", func() {
			svc := &corev1.Service{}
			result := mapper(ctx, svc)
			Expect(result).To(BeNil())
		})

		It("should return request for Service resource reference", func() {
			result := mapper(ctx, annotation)
			Expect(result).To(HaveLen(1))
			Expect(result[0].NamespacedName.Namespace).To(Equal("test-namespace"))
			Expect(result[0].NamespacedName.Name).To(Equal("test-object"))
		})

		It("should return nil for non-Service kind", func() {
			annotation.Spec.ResourceRef.Kind = "Pod"
			result := mapper(ctx, annotation)
			Expect(result).To(BeNil())
		})

		It("should return nil for wrong API version", func() {
			annotation.Spec.ResourceRef.APIVersion = "v2"
			result := mapper(ctx, annotation)
			Expect(result).To(BeNil())
		})
	})

})
