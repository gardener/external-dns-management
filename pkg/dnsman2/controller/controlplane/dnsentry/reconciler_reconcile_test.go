// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/common"
)

var _ = Describe("#hasMatchingWildcardEntry", func() {
	const namespace = "ns"

	entry := func(name string) client.Object {
		return &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       v1alpha1.DNSEntrySpec{DNSName: name},
		}
	}

	DescribeTable("wildcard shadowing",
		func(queried string, existing []client.Object, expected bool) {
			r := &entryReconciliation{
				EntryContext: common.EntryContext{
					Ctx:    context.Background(),
					Client: fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).WithObjects(existing...).Build(),
					Entry:  &v1alpha1.DNSEntry{Spec: v1alpha1.DNSEntrySpec{DNSName: queried}},
				},
				namespace: namespace,
			}

			got, res := r.hasMatchingWildcardEntry()
			Expect(res).To(BeNil())
			Expect(got).To(Equal(expected))
		},
		Entry("one label is shadowed", "foo.example.com", []client.Object{entry("*.example.com")}, true),
		Entry("bare domain is not shadowed", "example.com", []client.Object{entry("*.example.com")}, false),
		Entry("two labels are also shadowed", "a.b.example.com", []client.Object{entry("*.example.com")}, true),
		Entry("wildcard entry itself is not shadowed", "*.example.com", []client.Object{entry("*.example.com")}, false),
		Entry("different base domain does not match", "foo.example.com", []client.Object{entry("*.other.com")}, false),
		Entry("non-wildcard entry is ignored", "foo.example.com", []client.Object{entry("bar.example.com")}, false),
		Entry("bare asterisk entry does not match", "foo", []client.Object{entry("*")}, false),
		Entry("no entries", "foo.example.com", nil, false),
	)

	It("ignores wildcard entries in another namespace", func() {
		other := &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{Name: "wild", Namespace: "other"},
			Spec:       v1alpha1.DNSEntrySpec{DNSName: "*.example.com"},
		}
		r := &entryReconciliation{
			EntryContext: common.EntryContext{
				Ctx:    context.Background(),
				Client: fakeclient.NewClientBuilder().WithScheme(dnsmanclient.ClusterScheme).WithObjects(other).Build(),
				Entry:  &v1alpha1.DNSEntry{Spec: v1alpha1.DNSEntrySpec{DNSName: "foo.example.com"}},
			},
			namespace: namespace,
		}

		got, res := r.hasMatchingWildcardEntry()
		Expect(res).To(BeNil())
		Expect(got).To(BeFalse())
	})
})
