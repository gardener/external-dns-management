// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gateways_crd_watchdog_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	. "github.com/gardener/cert-management/pkg/certman2/controller/source/gateways_crd_watchdog"
)

var _ = Describe("Add", func() {
	Describe("#Predicate", func() {
		var (
			crdPredicate predicate.Predicate
			crd          *apiextensionsv1.CustomResourceDefinition

			test func(*apiextensionsv1.CustomResourceDefinition, types.GomegaMatcher)
		)

		BeforeEach(func() {
			crdPredicate = Predicate()

			crd = &apiextensionsv1.CustomResourceDefinition{}

			test = func(
				crd *apiextensionsv1.CustomResourceDefinition,
				match types.GomegaMatcher,
			) {
				Expect(crdPredicate.Create(event.CreateEvent{Object: crd})).To(match)
				Expect(crdPredicate.Update(event.UpdateEvent{ObjectOld: crd, ObjectNew: crd})).To(match)
				Expect(crdPredicate.Delete(event.DeleteEvent{Object: crd})).To(match)
				Expect(crdPredicate.Generic(event.GenericEvent{Object: crd})).To(BeFalse())
			}
		})

		It("should handle nil objects as expected", func() {
			test(nil, BeFalse())
		})

		It("should handle unmanaged objects as expected", func() {
			crd.Name = "foo"
			test(crd, BeFalse())
		})

		It("should handle relevant crd", func() {
			for _, name := range []string{
				"gateways.networking.istio.io",
				"virtualservices.networking.istio.io",
				"gateways.gateway.networking.k8s.io",
				"httproutes.gateway.networking.k8s.io",
			} {
				crd.Name = name
				test(crd, BeTrue())
			}
		})
	})
})
