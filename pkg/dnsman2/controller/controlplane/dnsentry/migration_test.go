// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

var _ = Describe("DNS Class Migration Logic", func() {
	DescribeTable("needsToMigrateDNSClassOrFinalizers",
		func(annotations map[string]string, finalizers []string, class string, secondaryClasses []string, expected bool) {
			entry := &v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Finalizers:  finalizers,
				},
			}

			r := &entryReconciliation{
				EntryContext: common.EntryContext{
					Class:            class,
					SecondaryClasses: secondaryClasses,
					Entry:            entry,
				},
			}

			Expect(r.needsToMigrateDNSClassOrFinalizers()).To(Equal(expected))
		},
		Entry("when secondary class finalizer is present", nil, []string{"class-b.dns.gardener.cloud/compound"}, "", []string{"class-b"}, true),
		Entry("when class annotation mismatches", map[string]string{dns.AnnotationClass: "class-a"}, []string{dns.FinalizerCompound}, "", nil, true),
		Entry("when no migration is needed", nil, []string{dns.FinalizerCompound}, "", []string{"class-b"}, false),
		Entry("when no migration is needed for non-default primary class",
			map[string]string{dns.AnnotationClass: "class-a"},
			[]string{dns.ClassFinalizerName("class-a")},
			"class-a",
			[]string{"class-b"},
			false,
		),
		Entry("when no migration is needed for non-default primary class without secondary classes",
			map[string]string{dns.AnnotationClass: "class-a"},
			[]string{dns.ClassFinalizerName("class-a")},
			"class-a",
			nil,
			false,
		),
	)
})
