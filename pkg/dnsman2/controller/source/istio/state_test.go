// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package istio_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/istio"
)

var _ = Describe("ObjectToGatewaysState", func() {
	var state *ObjectToGatewaysState

	BeforeEach(func() {
		state = NewObjectToGatewaysState()
	})

	Describe("Ingress mappings", func() {
		It("should return nil for an unknown ingress", func() {
			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "unknown"})).To(BeNil())
		})

		It("should add and retrieve a single ingress-to-gateway mapping", func() {
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})

			gateways := state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})
			Expect(gateways).To(ConsistOf(client.ObjectKey{Namespace: "ns", Name: "gateway1"}))
		})

		It("should add multiple gateways for the same ingress", func() {
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway2"})

			gateways := state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})
			Expect(gateways).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
				client.ObjectKey{Namespace: "ns", Name: "gateway2"},
			))
		})

		It("should not duplicate a gateway when added twice for the same ingress", func() {
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})

			gateways := state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})
			Expect(gateways).To(ConsistOf(client.ObjectKey{Namespace: "ns", Name: "gateway1"}))
		})

		It("should remove an ingress and all its gateway mappings", func() {
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway2"})
			state.RemoveIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})

			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})).To(BeNil())
		})

		It("should not affect other ingresses when removing one", func() {
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress2"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.RemoveIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})

			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})).To(BeNil())
			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress2"})).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
			))
		})
	})

	Describe("Service mappings", func() {
		It("should return nil for an unknown service", func() {
			Expect(state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "unknown"})).To(BeNil())
		})

		It("should add and retrieve a single service-to-gateway mapping", func() {
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})

			gateways := state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service1"})
			Expect(gateways).To(ConsistOf(client.ObjectKey{Namespace: "ns", Name: "gateway1"}))
		})

		It("should add multiple gateways for the same service", func() {
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway2"})

			gateways := state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service1"})
			Expect(gateways).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
				client.ObjectKey{Namespace: "ns", Name: "gateway2"},
			))
		})

		It("should not duplicate a gateway when added twice for the same service", func() {
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})

			gateways := state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service1"})
			Expect(gateways).To(ConsistOf(client.ObjectKey{Namespace: "ns", Name: "gateway1"}))
		})

		It("should remove a service and all its gateway mappings", func() {
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway2"})
			state.RemoveService(client.ObjectKey{Namespace: "ns", Name: "service1"})

			Expect(state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service1"})).To(BeNil())
		})

		It("should not affect other services when removing one", func() {
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service2"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.RemoveService(client.ObjectKey{Namespace: "ns", Name: "service1"})

			Expect(state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service1"})).To(BeNil())
			Expect(state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service2"})).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
			))
		})
	})

	Describe("#RemoveGateway", func() {
		It("should remove the gateway from all ingress and service mappings", func() {
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.RemoveGateway(client.ObjectKey{Namespace: "ns", Name: "gateway1"})

			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})).To(BeEmpty())
			Expect(state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service1"})).To(BeEmpty())
		})

		It("should not affect other gateways", func() {
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway2"})
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway2"})
			state.RemoveGateway(client.ObjectKey{Namespace: "ns", Name: "gateway1"})

			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway2"},
			))
			Expect(state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service1"})).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway2"},
			))
		})
	})

	Describe("#RemoveGatewayFromIngressMappings", func() {
		It("should remove the gateway only from ingress mappings", func() {
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.RemoveGatewayFromIngressMappings(client.ObjectKey{Namespace: "ns", Name: "gateway1"})

			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})).To(BeEmpty())
			Expect(state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service1"})).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
			))
		})

		It("should remove the gateway from all ingress entries", func() {
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress2"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.RemoveGatewayFromIngressMappings(client.ObjectKey{Namespace: "ns", Name: "gateway1"})

			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})).To(BeEmpty())
			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress2"})).To(BeEmpty())
		})
	})

	Describe("#RemoveGatewayFromServiceMappings", func() {
		It("should remove the gateway only from service mappings", func() {
			state.AddIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.RemoveGatewayFromServiceMappings(client.ObjectKey{Namespace: "ns", Name: "gateway1"})

			Expect(state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service1"})).To(BeEmpty())
			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns", Name: "ingress1"})).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
			))
		})

		It("should remove the gateway from all service entries", func() {
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service1"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.AddService(client.ObjectKey{Namespace: "ns", Name: "service2"}, client.ObjectKey{Namespace: "ns", Name: "gateway1"})
			state.RemoveGatewayFromServiceMappings(client.ObjectKey{Namespace: "ns", Name: "gateway1"})

			Expect(state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service1"})).To(BeEmpty())
			Expect(state.GetGatewaysForService(client.ObjectKey{Namespace: "ns", Name: "service2"})).To(BeEmpty())
		})
	})

	Describe("cross-namespace support", func() {
		It("should handle objects in different namespaces independently", func() {
			state.AddIngress(client.ObjectKey{Namespace: "ns1", Name: "ingress"}, client.ObjectKey{Namespace: "ns1", Name: "gateway"})

			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns1", Name: "ingress"})).To(ConsistOf(
				client.ObjectKey{Namespace: "ns1", Name: "gateway"},
			))
			Expect(state.GetGatewaysForIngress(client.ObjectKey{Namespace: "ns2", Name: "ingress"})).To(BeNil())
		})
	})
})
