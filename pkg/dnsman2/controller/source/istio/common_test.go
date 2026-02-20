// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package istio_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/testing"

	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/istio"
)

var _ = Describe("Common", func() {
	Describe("#DetermineAPIVersion", func() {
		var (
			dc *fakediscovery.FakeDiscovery
		)

		BeforeEach(func() {
			dc = &fakediscovery.FakeDiscovery{Fake: &testing.Fake{}}
			dc.Resources = []*metav1.APIResourceList{}
		})

		It("should return nil if there are no matching resources", func() {
			apiVersion, err := DetermineAPIVersion(dc)
			Expect(err).ToNot(HaveOccurred())
			Expect(apiVersion).To(BeNil())
		})

		It("should return nil if no matching kind is present", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "",
					APIResources: []metav1.APIResource{{Kind: "Pod"}, {Kind: "Deployment"}},
				},
			}
			apiVersion, err := DetermineAPIVersion(dc)
			Expect(err).ToNot(HaveOccurred())
			Expect(apiVersion).To(BeNil())
		})

		It("should return nil if Gateway is present but VirtualService is missing", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "networking.istio.io/v1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}},
				},
			}
			apiVersion, err := DetermineAPIVersion(dc)
			Expect(err).ToNot(HaveOccurred())
			Expect(apiVersion).To(BeNil())
		})

		It("should return nil if VirtualService is present but Gateway is missing", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "networking.istio.io/v1",
					APIResources: []metav1.APIResource{{Kind: "VirtualService"}},
				},
			}
			apiVersion, err := DetermineAPIVersion(dc)
			Expect(err).ToNot(HaveOccurred())
			Expect(apiVersion).To(BeNil())
		})

		It("should return v1alpha3 if both Gateway and VirtualService are present in the corresponding API version", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "networking.istio.io/v1alpha3",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "VirtualService"}},
				},
			}
			apiVersion, err := DetermineAPIVersion(dc)
			Expect(err).ToNot(HaveOccurred())
			Expect(apiVersion).NotTo(BeNil())
			Expect(string(*apiVersion)).To(Equal("v1alpha3"))
		})

		It("should return v1beta1 if both Gateway and VirtualService are present in the corresponding API version", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "networking.istio.io/v1beta1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "VirtualService"}},
				},
			}
			apiVersion, err := DetermineAPIVersion(dc)
			Expect(err).ToNot(HaveOccurred())
			Expect(apiVersion).NotTo(BeNil())
			Expect(string(*apiVersion)).To(Equal("v1beta1"))
		})

		It("should return v1 if both Gateway and VirtualService are present in the corresponding API version", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "networking.istio.io/v1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "VirtualService"}},
				},
			}
			apiVersion, err := DetermineAPIVersion(dc)
			Expect(err).ToNot(HaveOccurred())
			Expect(apiVersion).NotTo(BeNil())
			Expect(string(*apiVersion)).To(Equal("v1"))
		})

		It("should prefer v1 over v1alpha3 and v1beta1", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "networking.istio.io/v1alpha3",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "VirtualService"}},
				},
				{
					GroupVersion: "networking.istio.io/v1beta1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "VirtualService"}},
				},
				{
					GroupVersion: "networking.istio.io/v1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "VirtualService"}},
				},
			}
			apiVersion, err := DetermineAPIVersion(dc)
			Expect(err).ToNot(HaveOccurred())
			Expect(apiVersion).NotTo(BeNil())
			Expect(string(*apiVersion)).To(Equal("v1"))
		})

		It("should prefer v1beta1 over v1alpha3", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "networking.istio.io/v1alpha3",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "VirtualService"}},
				},
				{
					GroupVersion: "networking.istio.io/v1beta1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "VirtualService"}},
				},
			}
			apiVersion, err := DetermineAPIVersion(dc)
			Expect(err).ToNot(HaveOccurred())
			Expect(apiVersion).NotTo(BeNil())
			Expect(string(*apiVersion)).To(Equal("v1beta1"))
		})
	})

	Describe("#MapGatewayNamesToRequest", func() {
		It("returns no requests when given no gateway names", func() {
			Expect(MapGatewayNamesToRequest([]string{}, "default")).To(BeEmpty())
		})

		It("maps a fully qualified gateway name to reconcile request", func() {
			gatewayNames := []string{"my-namespace/my-gateway"}
			actual := MapGatewayNamesToRequest(gatewayNames, "default")
			Expect(actual).To(HaveLen(1))
			Expect(actual[0].NamespacedName.String()).To(Equal("my-namespace/my-gateway"))
		})

		It("uses the default namespace if the gateway name does not specify one", func() {
			gatewayNames := []string{"my-gateway"}
			actual := MapGatewayNamesToRequest(gatewayNames, "default")
			Expect(actual).To(HaveLen(1))
			Expect(actual[0].NamespacedName.String()).To(Equal("default/my-gateway"))
		})

		It("omits gateway names that don't match the expected format", func() {
			gatewayNames := []string{"too/many/parts"}
			actual := MapGatewayNamesToRequest(gatewayNames, "default")
			Expect(actual).To(BeEmpty())
		})

		It("maps multiple gateway names to reconcile requests", func() {
			gatewayNames := []string{"my-namespace/my-gateway", "my-gateway", "too/many/parts"}
			actual := MapGatewayNamesToRequest(gatewayNames, "default")
			Expect(actual).To(HaveLen(2))
			Expect(actual[0].NamespacedName.String()).To(Equal("my-namespace/my-gateway"))
			Expect(actual[1].NamespacedName.String()).To(Equal("default/my-gateway"))
		})
	})
})
