// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crdwatch_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/crdwatch"
	gatewayapiv1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi/v1"
	gatewayapiv1beta1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi/v1beta1"
)

var _ = Describe("Reconciler", func() {
	Describe("#Reconcile", func() {
		var (
			exitCalled bool
			exitCode   int
			exit       = func(code int) {
				exitCalled = true
				exitCode = code
			}
			dc         *fake.FakeDiscovery
			reconciler crdwatch.Reconciler
		)

		BeforeEach(func() {
			gatewayapiv1beta1.Activated = false
			gatewayapiv1.Activated = false

			exitCalled = false
			exitCode = -1
			dc = &fake.FakeDiscovery{Fake: &testing.Fake{}}
			dc.Resources = []*metav1.APIResourceList{}
			reconciler = crdwatch.Reconciler{
				Discovery: dc,
				Exit:      exit,
			}
		})

		It("should not exit with no CRDs and inactive source controllers", func() {
			doReconcile(reconciler)
			Expect(exitCalled).To(BeFalse())
		})

		It("should exit with no CRDs and active v1beta1 Gateway API source controller", func() {
			gatewayapiv1beta1.Activated = true
			doReconcile(reconciler)
			Expect(exitCalled).To(BeTrue())
			Expect(exitCode).To(Equal(3))
		})

		It("should exit with no CRDs and active v1 Gateway API source controller", func() {
			gatewayapiv1.Activated = true
			doReconcile(reconciler)
			Expect(exitCalled).To(BeTrue())
			Expect(exitCode).To(Equal(3))
		})

		It("should exit with v1beta1 Gateway API CRDs and inactive source controller", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "gateway.networking.k8s.io/v1beta1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "HTTPRoute"}},
				},
			}
			doReconcile(reconciler)
			Expect(exitCalled).To(BeTrue())
			Expect(exitCode).To(Equal(3))
		})

		It("should exit with v1 Gateway API CRDs and inactive source controller", func() {
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "gateway.networking.k8s.io/v1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "HTTPRoute"}},
				},
			}
			doReconcile(reconciler)
			Expect(exitCalled).To(BeTrue())
			Expect(exitCode).To(Equal(3))
		})

		It("should not exit with v1beta1 Gateway API CRDs and active source controller", func() {
			gatewayapiv1beta1.Activated = true
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "gateway.networking.k8s.io/v1beta1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "HTTPRoute"}},
				},
			}
			doReconcile(reconciler)
			Expect(exitCalled).To(BeFalse())
		})

		It("should not exit with v1 Gateway API CRDs and active source controller", func() {
			gatewayapiv1.Activated = true
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "gateway.networking.k8s.io/v1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "HTTPRoute"}},
				},
			}
			doReconcile(reconciler)
			Expect(exitCalled).To(BeFalse())
		})

		It("should exit with both v1beta1 and v1 Gateway API CRDs and inactive v1 source controller", func() {
			gatewayapiv1beta1.Activated = true
			dc.Resources = []*metav1.APIResourceList{
				{
					GroupVersion: "gateway.networking.k8s.io/v1beta1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "HTTPRoute"}},
				},
				{
					GroupVersion: "gateway.networking.k8s.io/v1",
					APIResources: []metav1.APIResource{{Kind: "Gateway"}, {Kind: "HTTPRoute"}},
				},
			}
			doReconcile(reconciler)
			Expect(exitCalled).To(BeTrue())
			Expect(exitCode).To(Equal(3))
		})
	})
})

func doReconcile(reconciler crdwatch.Reconciler) {
	GinkgoHelper()
	_, err := reconciler.Reconcile(context.Background(), reconcile.Request{})
	Expect(err).ToNot(HaveOccurred())
}
