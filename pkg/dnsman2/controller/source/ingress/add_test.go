// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ingress_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/ingress"
)

var _ = Describe("Add", func() {
	Describe("#IsRelevantSourceObject", func() {
		var (
			ing *networkingv1.Ingress

			actuator   = &ingress.Actuator{}
			reconciler = common.NewSourceReconciler(actuator)
			test       = func(ing *networkingv1.Ingress, match types.GomegaMatcher) {
				Expect(actuator.IsRelevantSourceObject(reconciler, ing)).To(match)
			}
		)

		BeforeEach(func() {
			ing = &networkingv1.Ingress{}
		})

		It("should handle nil objects as expected", func() {
			test(nil, BeFalse())
		})

		It("should handle empty objects as expected", func() {
			test(ing, BeFalse())
		})

		It("should handle an ingress with annotations as expected", func() {
			ing.Annotations = map[string]string{
				"dns.gardener.cloud/class":    "gardendns",
				"dns.gardener.cloud/dnsnames": "example.com",
			}
			test(ing, BeTrue())
		})

		It("should handle an ingress with missing DNS names annotation as expected", func() {
			ing.Annotations = map[string]string{
				"dns.gardener.cloud/class": "gardendns",
			}
			test(ing, BeFalse())
		})

		It("should handle an ingress with a wrong DNS class annotation as expected", func() {
			ing.Annotations = map[string]string{
				"dns.gardener.cloud/class": "jardindns",
			}
			test(ing, BeFalse())
		})
	})
})
