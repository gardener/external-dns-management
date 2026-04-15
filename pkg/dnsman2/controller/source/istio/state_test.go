// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package istio_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	istionetworkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/istio"
)

func newIngress(namespace, name string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func newService(namespace, name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func newGateway(namespace, name string) *istionetworkingv1.Gateway {
	return &istionetworkingv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

var _ = Describe("ObjectToGatewaysState", func() {
	var state *ObjectToGatewaysState

	BeforeEach(func() {
		state = NewObjectToGatewaysState()
	})

	Describe("Ingress mappings", func() {
		It("should return nil for an unknown ingress", func() {
			ingress := newIngress("ns", "unknown")
			Expect(state.GetGatewaysForIngress(ingress)).To(BeNil())
		})

		It("should add and retrieve a single ingress-to-gateway mapping", func() {
			ingress := newIngress("ns", "ingress1")
			gateway := newGateway("ns", "gateway1")

			state.AddIngress(ingress, gateway)

			gateways := state.GetGatewaysForIngress(ingress)
			Expect(gateways).To(ConsistOf(client.ObjectKey{Namespace: "ns", Name: "gateway1"}))
		})

		It("should add multiple gateways for the same ingress", func() {
			ingress := newIngress("ns", "ingress1")
			gateway1 := newGateway("ns", "gateway1")
			gateway2 := newGateway("ns", "gateway2")

			state.AddIngress(ingress, gateway1)
			state.AddIngress(ingress, gateway2)

			gateways := state.GetGatewaysForIngress(ingress)
			Expect(gateways).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
				client.ObjectKey{Namespace: "ns", Name: "gateway2"},
			))
		})

		It("should not duplicate a gateway when added twice for the same ingress", func() {
			ingress := newIngress("ns", "ingress1")
			gateway := newGateway("ns", "gateway1")

			state.AddIngress(ingress, gateway)
			state.AddIngress(ingress, gateway)

			gateways := state.GetGatewaysForIngress(ingress)
			Expect(gateways).To(ConsistOf(client.ObjectKey{Namespace: "ns", Name: "gateway1"}))
		})

		It("should remove an ingress and all its gateway mappings", func() {
			ingress := newIngress("ns", "ingress1")
			gateway1 := newGateway("ns", "gateway1")
			gateway2 := newGateway("ns", "gateway2")

			state.AddIngress(ingress, gateway1)
			state.AddIngress(ingress, gateway2)
			state.RemoveIngress(ingress)

			Expect(state.GetGatewaysForIngress(ingress)).To(BeNil())
		})

		It("should not affect other ingresses when removing one", func() {
			ingress1 := newIngress("ns", "ingress1")
			ingress2 := newIngress("ns", "ingress2")
			gateway := newGateway("ns", "gateway1")

			state.AddIngress(ingress1, gateway)
			state.AddIngress(ingress2, gateway)
			state.RemoveIngress(ingress1)

			Expect(state.GetGatewaysForIngress(ingress1)).To(BeNil())
			Expect(state.GetGatewaysForIngress(ingress2)).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
			))
		})
	})

	Describe("Service mappings", func() {
		It("should return nil for an unknown service", func() {
			service := newService("ns", "unknown")
			Expect(state.GetGatewaysForService(service)).To(BeNil())
		})

		It("should add and retrieve a single service-to-gateway mapping", func() {
			service := newService("ns", "service1")
			gateway := newGateway("ns", "gateway1")

			state.AddService(service, gateway)

			gateways := state.GetGatewaysForService(service)
			Expect(gateways).To(ConsistOf(client.ObjectKey{Namespace: "ns", Name: "gateway1"}))
		})

		It("should add multiple gateways for the same service", func() {
			service := newService("ns", "service1")
			gateway1 := newGateway("ns", "gateway1")
			gateway2 := newGateway("ns", "gateway2")

			state.AddService(service, gateway1)
			state.AddService(service, gateway2)

			gateways := state.GetGatewaysForService(service)
			Expect(gateways).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
				client.ObjectKey{Namespace: "ns", Name: "gateway2"},
			))
		})

		It("should not duplicate a gateway when added twice for the same service", func() {
			service := newService("ns", "service1")
			gateway := newGateway("ns", "gateway1")

			state.AddService(service, gateway)
			state.AddService(service, gateway)

			gateways := state.GetGatewaysForService(service)
			Expect(gateways).To(ConsistOf(client.ObjectKey{Namespace: "ns", Name: "gateway1"}))
		})

		It("should remove a service and all its gateway mappings", func() {
			service := newService("ns", "service1")
			gateway1 := newGateway("ns", "gateway1")
			gateway2 := newGateway("ns", "gateway2")

			state.AddService(service, gateway1)
			state.AddService(service, gateway2)
			state.RemoveService(service)

			Expect(state.GetGatewaysForService(service)).To(BeNil())
		})

		It("should not affect other services when removing one", func() {
			service1 := newService("ns", "service1")
			service2 := newService("ns", "service2")
			gateway := newGateway("ns", "gateway1")

			state.AddService(service1, gateway)
			state.AddService(service2, gateway)
			state.RemoveService(service1)

			Expect(state.GetGatewaysForService(service1)).To(BeNil())
			Expect(state.GetGatewaysForService(service2)).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
			))
		})
	})

	Describe("#RemoveGateway", func() {
		It("should remove the gateway from all ingress and service mappings", func() {
			ingress := newIngress("ns", "ingress1")
			service := newService("ns", "service1")
			gateway := newGateway("ns", "gateway1")

			state.AddIngress(ingress, gateway)
			state.AddService(service, gateway)
			state.RemoveGateway(gateway)

			Expect(state.GetGatewaysForIngress(ingress)).To(BeEmpty())
			Expect(state.GetGatewaysForService(service)).To(BeEmpty())
		})

		It("should not affect other gateways", func() {
			ingress := newIngress("ns", "ingress1")
			service := newService("ns", "service1")
			gateway1 := newGateway("ns", "gateway1")
			gateway2 := newGateway("ns", "gateway2")

			state.AddIngress(ingress, gateway1)
			state.AddIngress(ingress, gateway2)
			state.AddService(service, gateway1)
			state.AddService(service, gateway2)
			state.RemoveGateway(gateway1)

			Expect(state.GetGatewaysForIngress(ingress)).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway2"},
			))
			Expect(state.GetGatewaysForService(service)).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway2"},
			))
		})
	})

	Describe("#RemoveGatewayFromIngressMappings", func() {
		It("should remove the gateway only from ingress mappings", func() {
			ingress := newIngress("ns", "ingress1")
			service := newService("ns", "service1")
			gateway := newGateway("ns", "gateway1")

			state.AddIngress(ingress, gateway)
			state.AddService(service, gateway)
			state.RemoveGatewayFromIngressMappings(gateway)

			Expect(state.GetGatewaysForIngress(ingress)).To(BeEmpty())
			Expect(state.GetGatewaysForService(service)).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
			))
		})

		It("should remove the gateway from all ingress entries", func() {
			ingress1 := newIngress("ns", "ingress1")
			ingress2 := newIngress("ns", "ingress2")
			gateway := newGateway("ns", "gateway1")

			state.AddIngress(ingress1, gateway)
			state.AddIngress(ingress2, gateway)
			state.RemoveGatewayFromIngressMappings(gateway)

			Expect(state.GetGatewaysForIngress(ingress1)).To(BeEmpty())
			Expect(state.GetGatewaysForIngress(ingress2)).To(BeEmpty())
		})
	})

	Describe("#RemoveGatewayFromServiceMappings", func() {
		It("should remove the gateway only from service mappings", func() {
			ingress := newIngress("ns", "ingress1")
			service := newService("ns", "service1")
			gateway := newGateway("ns", "gateway1")

			state.AddIngress(ingress, gateway)
			state.AddService(service, gateway)
			state.RemoveGatewayFromServiceMappings(gateway)

			Expect(state.GetGatewaysForService(service)).To(BeEmpty())
			Expect(state.GetGatewaysForIngress(ingress)).To(ConsistOf(
				client.ObjectKey{Namespace: "ns", Name: "gateway1"},
			))
		})

		It("should remove the gateway from all service entries", func() {
			service1 := newService("ns", "service1")
			service2 := newService("ns", "service2")
			gateway := newGateway("ns", "gateway1")

			state.AddService(service1, gateway)
			state.AddService(service2, gateway)
			state.RemoveGatewayFromServiceMappings(gateway)

			Expect(state.GetGatewaysForService(service1)).To(BeEmpty())
			Expect(state.GetGatewaysForService(service2)).To(BeEmpty())
		})
	})

	Describe("cross-namespace support", func() {
		It("should handle objects in different namespaces independently", func() {
			ingress1 := newIngress("ns1", "ingress")
			ingress2 := newIngress("ns2", "ingress")
			gateway := newGateway("ns1", "gateway")

			state.AddIngress(ingress1, gateway)

			Expect(state.GetGatewaysForIngress(ingress1)).To(ConsistOf(
				client.ObjectKey{Namespace: "ns1", Name: "gateway"},
			))
			Expect(state.GetGatewaysForIngress(ingress2)).To(BeNil())
		})
	})
})
