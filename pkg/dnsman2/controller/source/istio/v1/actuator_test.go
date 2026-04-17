// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	istionetworkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	v1actuator "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/istio/v1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

var _ = Describe("Actuator", func() {
	var actuator *v1actuator.Actuator

	BeforeEach(func() {
		actuator = v1actuator.NewActuator(nil)
	})

	Describe("#ControllerName", func() {
		It("should return the controller name", func() {
			Expect(actuator.ControllerName()).To(Equal("istiov1-source"))
		})
	})

	Describe("#FinalizerLocalName", func() {
		It("should return the finalizer local name", func() {
			Expect(actuator.FinalizerLocalName()).To(Equal("istio-dns"))
		})
	})

	Describe("#GetGVK", func() {
		It("should return the Istio v1 Gateway GVK", func() {
			gvk := actuator.GetGVK()
			Expect(gvk).To(Equal(schema.GroupVersionKind{
				Group:   "networking.istio.io",
				Version: "v1",
				Kind:    "Gateway",
			}))
		})
	})

	Describe("#NewSourceObject", func() {
		It("should return a new empty Gateway", func() {
			obj := actuator.NewSourceObject()
			Expect(obj).NotTo(BeNil())
			Expect(obj).To(BeAssignableToTypeOf(&istionetworkingv1.Gateway{}))
		})
	})

	Describe("#ShouldSetTargetEntryAnnotation", func() {
		It("should return false", func() {
			Expect(actuator.ShouldSetTargetEntryAnnotation()).To(BeFalse())
		})
	})

	Describe("#OnDelete", func() {
		It("should not panic when removing a gateway", func() {
			Expect(func() { actuator.OnDelete(client.ObjectKey{Namespace: "ns", Name: "gw1"}) }).NotTo(Panic())
		})
	})

	Describe("#ShouldActivate", func() {
		var dc *fakediscovery.FakeDiscovery

		BeforeEach(func() {
			dc = &fakediscovery.FakeDiscovery{Fake: &testing.Fake{}}
			dc.Resources = []*metav1.APIResourceList{}
			actuator = v1actuator.NewActuator(dc)
		})

		It("should return true when Istio v1 Gateway and VirtualService CRDs are present", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "networking.istio.io/v1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "VirtualService"}},
				},
			}
			activated, err := actuator.ShouldActivate()
			Expect(err).NotTo(HaveOccurred())
			Expect(activated).To(BeTrue())
		})

		It("should return false when no Istio CRDs are present", func() {
			activated, err := actuator.ShouldActivate()
			Expect(err).NotTo(HaveOccurred())
			Expect(activated).To(BeFalse())
		})

		It("should return false when only v1beta1 CRDs are present", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "networking.istio.io/v1beta1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "VirtualService"}},
				},
			}
			activated, err := actuator.ShouldActivate()
			Expect(err).NotTo(HaveOccurred())
			Expect(activated).To(BeFalse())
		})

		It("should return false when only v1alpha3 CRDs are present", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "networking.istio.io/v1alpha3",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "VirtualService"}},
				},
			}
			activated, err := actuator.ShouldActivate()
			Expect(err).NotTo(HaveOccurred())
			Expect(activated).To(BeFalse())
		})
	})

	Describe("#IsRelevantSourceObject", func() {
		var r *common.SourceReconciler[*istionetworkingv1.Gateway]

		BeforeEach(func() {
			r = &common.SourceReconciler[*istionetworkingv1.Gateway]{
				GVK:   istionetworkingv1.SchemeGroupVersion.WithKind("Gateway"),
				State: state.GetState().GetAnnotationState(),
			}
		})

		It("should return false for a nil gateway", func() {
			Expect(actuator.IsRelevantSourceObject(r, nil)).To(BeFalse())
		})

		It("should return false when DNS names annotation is missing", func() {
			gateway := &istionetworkingv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "ns",
					Name:        "gw1",
					Annotations: map[string]string{},
				},
			}
			Expect(actuator.IsRelevantSourceObject(r, gateway)).To(BeFalse())
		})

		It("should return true when DNS names annotation is present and class matches", func() {
			gateway := &istionetworkingv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "gw1",
					Annotations: map[string]string{
						dns.AnnotationDNSNames: "example.com",
					},
				},
			}
			Expect(actuator.IsRelevantSourceObject(r, gateway)).To(BeTrue())
		})

		It("should return false when class does not match", func() {
			r.SourceClass = "other-class"
			gateway := &istionetworkingv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "gw1",
					Annotations: map[string]string{
						dns.AnnotationDNSNames: "example.com",
						dns.AnnotationClass:    "default-class",
					},
				},
			}
			Expect(actuator.IsRelevantSourceObject(r, gateway)).To(BeFalse())
		})

		It("should return true when class annotation matches source class", func() {
			r.SourceClass = "my-class"
			gateway := &istionetworkingv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "gw1",
					Annotations: map[string]string{
						dns.AnnotationDNSNames: "example.com",
						dns.AnnotationClass:    "my-class",
					},
				},
			}
			Expect(actuator.IsRelevantSourceObject(r, gateway)).To(BeTrue())
		})
	})
})
