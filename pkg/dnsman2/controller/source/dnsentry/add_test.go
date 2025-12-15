// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	. "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/dnsentry"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("Add", func() {
	Describe("#IsRelevantSourceObject", func() {
		var (
			entry *dnsv1alpha1.DNSEntry

			actuator   = &Actuator{}
			reconciler = common.NewSourceReconciler(actuator)
			test       = func(entry *dnsv1alpha1.DNSEntry, match types.GomegaMatcher) {
				GinkgoHelper()
				Expect(actuator.IsRelevantSourceObject(reconciler, entry)).To(match)
			}
		)

		BeforeEach(func() {
			entry = &dnsv1alpha1.DNSEntry{}
		})

		It("should handle nil objects as expected", func() {
			test(nil, BeFalse())
		})

		It("should handle empty objects as expected", func() {
			test(entry, BeTrue())
		})

		It("should handle entry with DNS name as expected", func() {
			entry.Spec.DNSName = "foo.example.com"
			test(entry, BeTrue())
		})

		It("should handle services of wrong class as expected", func() {
			entry.Spec.DNSName = "foo.example.com"
			entry.Annotations = map[string]string{dns.AnnotationClass: "bar"}
			test(entry, BeFalse())
			entry.Annotations = map[string]string{dns.AnnotationClass: dns.DefaultClass}
			test(entry, BeTrue())
		})
	})
})
