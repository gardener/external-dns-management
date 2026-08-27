// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package providerselector

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsprovider"
)

func buildFakeClient(scheme *runtime.Scheme, objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		WithIndex(&v1alpha1.DNSEntry{}, dnsprovider.EntryStatusProvider,
			func(obj client.Object) []string {
				entry := obj.(*v1alpha1.DNSEntry)
				return []string{ptr.Deref(entry.Status.Provider, "")}
			}).
		Build()
}

var _ = Describe("ListEntriesForProvider", func() {
	var (
		ctx         context.Context
		namespace   string
		providerKey client.ObjectKey
		scheme      = runtime.NewScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
		Expect(v1alpha1.SchemeBuilder.AddToScheme(scheme)).To(Succeed())
		namespace = "default"
		providerKey = client.ObjectKey{Namespace: namespace, Name: "test-provider"}
	})

	It("should return empty list when no entries exist", func() {
		entries, err := ListEntriesForProvider(ctx, buildFakeClient(scheme), namespace, providerKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})

	It("should list entries with provider set", func() {
		providerName := providerKey.String()
		fakeClient := buildFakeClient(scheme,
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry1", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: new(providerName)},
			},
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry2", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: new(providerName)},
			},
		)

		entries, err := ListEntriesForProvider(ctx, fakeClient, namespace, providerKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(2))
	})

	It("should only list entries with status.provider set", func() {
		providerName := providerKey.String()
		fakeClient := buildFakeClient(scheme,
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry1", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: new(providerName)},
			},
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry2", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: nil},
			},
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry3", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{},
			},
		)

		entries, err := ListEntriesForProvider(ctx, fakeClient, namespace, providerKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Name).To(Equal("entry1"))
	})

	It("should only list entries for the specified provider", func() {
		providerName := providerKey.String()
		fakeClient := buildFakeClient(scheme,
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry1", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: new(providerName)},
			},
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry2", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: new("default/other-provider")},
			},
		)

		entries, err := ListEntriesForProvider(ctx, fakeClient, namespace, providerKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Name).To(Equal("entry1"))
	})
})

var _ = Describe("quotaExceededError", func() {
	It("should format error message correctly", func() {
		providerKey := client.ObjectKey{Namespace: "default", Name: "test-provider"}
		err := &quotaExceededError{
			providerKey: providerKey,
			quota:       3,
		}

		expectedMsg := "provider default/test-provider has reached its entries quota (max=3)"
		Expect(err.Error()).To(Equal(expectedMsg))
	})
})
