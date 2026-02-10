// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crdwatch

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var _ = Describe("Add", func() {
	Describe("#relevantCRDPredicate", func() {
		var (
			predicate  = relevantCRDPredicate()
			gatewayObj = &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateways.gateway.networking.k8s.io",
				},
			}
			httpRouteObj = &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "httproutes.gateway.networking.k8s.io",
				},
			}
			istioGatewayObj = &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gateways.networking.istio.io",
				},
			}
			istioVirtualServiceObj = &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "virtualservices.networking.istio.io",
				},
			}
			irrelevantObj = &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "grpcroutes.gateway.networking.k8s.io",
				},
			}
		)

		It("should handle create events appropriately", func() {
			Expect(predicate.CreateFunc(event.CreateEvent{Object: gatewayObj})).To(BeTrue())
			Expect(predicate.CreateFunc(event.CreateEvent{Object: httpRouteObj})).To(BeTrue())

			Expect(predicate.CreateFunc(event.CreateEvent{Object: istioGatewayObj})).To(BeTrue())
			Expect(predicate.CreateFunc(event.CreateEvent{Object: istioVirtualServiceObj})).To(BeTrue())

			Expect(predicate.CreateFunc(event.CreateEvent{Object: irrelevantObj})).To(BeFalse())
		})

		It("should handle update events appropriately", func() {
			Expect(predicate.UpdateFunc(event.UpdateEvent{ObjectNew: gatewayObj})).To(BeTrue())
			Expect(predicate.UpdateFunc(event.UpdateEvent{ObjectNew: httpRouteObj})).To(BeTrue())

			Expect(predicate.UpdateFunc(event.UpdateEvent{ObjectNew: istioGatewayObj})).To(BeTrue())
			Expect(predicate.UpdateFunc(event.UpdateEvent{ObjectNew: istioVirtualServiceObj})).To(BeTrue())

			Expect(predicate.UpdateFunc(event.UpdateEvent{ObjectNew: irrelevantObj})).To(BeFalse())
		})

		It("should handle delete events appropriately", func() {
			Expect(predicate.DeleteFunc(event.DeleteEvent{Object: gatewayObj})).To(BeTrue())
			Expect(predicate.DeleteFunc(event.DeleteEvent{Object: httpRouteObj})).To(BeTrue())

			Expect(predicate.DeleteFunc(event.DeleteEvent{Object: istioGatewayObj})).To(BeTrue())
			Expect(predicate.DeleteFunc(event.DeleteEvent{Object: istioVirtualServiceObj})).To(BeTrue())

			Expect(predicate.DeleteFunc(event.DeleteEvent{Object: irrelevantObj})).To(BeFalse())
		})

		It("should return false for generics events", func() {
			Expect(predicate.GenericFunc(event.GenericEvent{Object: httpRouteObj})).To(BeFalse())
		})
	})

	DescribeTable("#isRelevantCRD", func(crdName string, expected bool) {
		Expect(isRelevantCRD(crdName)).To(Equal(expected))
	},
		Entry("relevant Gateway CRD", "gateways.gateway.networking.k8s.io", true),
		Entry("relevant HTTPRoute CRD", "httproutes.gateway.networking.k8s.io", true),

		Entry("relevant Istio Gateway CRD", "gateways.networking.istio.io", true),
		Entry("relevant Istio VirtualService CRD", "virtualservices.networking.istio.io", true),

		Entry("irrelevant CRD", "grpcroutes.gateway.networking.k8s.io", false),
	)
})
