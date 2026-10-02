// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package providerselector

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/common"
	dnsman2 "github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

const ownersAnnotation = dnsman2.AnnotationOwners

func withOwners(obj metav1.Object, owners string) {
	obj.SetAnnotations(map[string]string{ownersAnnotation: owners})
}

var _ = Describe("getNamespaceFromOwner", func() {
	It("should return empty string when no owner annotation is present", func() {
		provider := &v1alpha1.DNSProvider{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "target"}}
		Expect(getNamespaceFromOwner(provider)).To(BeEmpty())
	})

	It("should extract the namespace from a simple owner reference", func() {
		provider := &v1alpha1.DNSProvider{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "target"}}
		withOwners(provider, "dns.gardener.cloud/DNSProvider/shoot-ns/original")
		Expect(getNamespaceFromOwner(provider)).To(Equal("shoot-ns"))
	})

	It("should extract the namespace from an owner reference with a cluster ID prefix", func() {
		provider := &v1alpha1.DNSProvider{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "target"}}
		withOwners(provider, "source-cluster-id:dns.gardener.cloud/DNSProvider/shoot-ns/original")
		Expect(getNamespaceFromOwner(provider)).To(Equal("shoot-ns"))
	})

	It("should use the first owner reference when multiple are present", func() {
		provider := &v1alpha1.DNSProvider{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "target"}}
		withOwners(provider, "dns.gardener.cloud/DNSProvider/first-ns/first,dns.gardener.cloud/DNSProvider/second-ns/second")
		Expect(getNamespaceFromOwner(provider)).To(Equal("first-ns"))
	})

	It("should return empty string when the owner reference has too few parts", func() {
		provider := &v1alpha1.DNSProvider{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "target"}}
		withOwners(provider, "only-one-part")
		Expect(getNamespaceFromOwner(provider)).To(BeEmpty())
	})
})

var _ = Describe("filterByOwnerNamespaceRestriction", func() {
	var (
		entry    *v1alpha1.DNSEntry
		selector *providerSelector

		newSelector = func(restricted bool) *providerSelector {
			return &providerSelector{
				EntryContext: common.EntryContext{
					Log:   GinkgoLogr,
					Entry: entry,
				},
				ownerNamespaceRestricted: restricted,
			}
		}

		newProvider = func(name, owners string) v1alpha1.DNSProvider {
			p := v1alpha1.DNSProvider{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "target"}}
			if owners != "" {
				withOwners(&p, owners)
			}
			return p
		}
	)

	BeforeEach(func() {
		entry = &v1alpha1.DNSEntry{ObjectMeta: metav1.ObjectMeta{Name: "entry", Namespace: "target"}}
	})

	It("should not filter anything when restriction is disabled", func() {
		selector = newSelector(false)
		providers := []v1alpha1.DNSProvider{
			newProvider("p1", "dns.gardener.cloud/DNSEntry/other-ns/svc"),
		}

		allowed, excluded := selector.filterByOwnerNamespaceRestriction(providers)
		Expect(allowed).To(Equal(providers))
		Expect(excluded).To(BeNil())
	})

	It("should not filter anything when the entry has no owner namespace", func() {
		selector = newSelector(true) // entry has no owner annotation
		providers := []v1alpha1.DNSProvider{
			newProvider("p1", "dns.gardener.cloud/DNSProvider/other-ns/original"),
		}

		allowed, excluded := selector.filterByOwnerNamespaceRestriction(providers)
		Expect(allowed).To(Equal(providers))
		Expect(excluded).To(BeNil())
	})

	Context("when restriction is enabled and the entry has an owner namespace", func() {
		BeforeEach(func() {
			withOwners(entry, "dns.gardener.cloud/Service/shoot-ns/my-service")
			selector = newSelector(true)
		})

		It("should allow providers whose owner namespace matches the entry owner namespace", func() {
			providers := []v1alpha1.DNSProvider{
				newProvider("p1", "dns.gardener.cloud/DNSProvider/shoot-ns/original"),
			}

			allowed, excluded := selector.filterByOwnerNamespaceRestriction(providers)
			Expect(allowed).To(HaveLen(1))
			Expect(allowed[0].Name).To(Equal("p1"))
			Expect(excluded).To(BeNil())
		})

		It("should allow providers without an owner namespace (non-replicated providers)", func() {
			providers := []v1alpha1.DNSProvider{
				newProvider("p1", ""),
			}

			allowed, excluded := selector.filterByOwnerNamespaceRestriction(providers)
			Expect(allowed).To(HaveLen(1))
			Expect(allowed[0].Name).To(Equal("p1"))
			Expect(excluded).To(BeNil())
		})

		It("should exclude providers whose owner namespace differs from the entry owner namespace", func() {
			providers := []v1alpha1.DNSProvider{
				newProvider("p1", "dns.gardener.cloud/DNSProvider/other-ns/original"),
			}

			allowed, excluded := selector.filterByOwnerNamespaceRestriction(providers)
			Expect(allowed).To(BeNil())
			Expect(excluded).To(HaveLen(1))
			Expect(excluded[0].Name).To(Equal("p1"))
		})

		It("should partition mixed providers into allowed and excluded", func() {
			providers := []v1alpha1.DNSProvider{
				newProvider("same", "dns.gardener.cloud/DNSProvider/shoot-ns/original"),
				newProvider("other", "dns.gardener.cloud/DNSProvider/other-ns/original"),
				newProvider("unowned", ""),
			}

			allowed, excluded := selector.filterByOwnerNamespaceRestriction(providers)
			Expect(allowed).To(HaveLen(2))
			Expect([]string{allowed[0].Name, allowed[1].Name}).To(ConsistOf("same", "unowned"))
			Expect(excluded).To(HaveLen(1))
			Expect(excluded[0].Name).To(Equal("other"))
		})

		It("should match the entry owner namespace even when the provider owner uses a cluster ID prefix", func() {
			providers := []v1alpha1.DNSProvider{
				newProvider("p1", "source-cluster-id:dns.gardener.cloud/DNSProvider/shoot-ns/original"),
			}

			allowed, excluded := selector.filterByOwnerNamespaceRestriction(providers)
			Expect(allowed).To(HaveLen(1))
			Expect(excluded).To(BeNil())
		})
	})
})
