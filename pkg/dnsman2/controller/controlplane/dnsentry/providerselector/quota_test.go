// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
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

var _ = Describe("CountEntriesForProvider", func() {
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

	It("should return 0 when no entries exist", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithIndex(&v1alpha1.DNSEntry{}, dnsprovider.EntryStatusProvider,
				func(obj client.Object) []string {
					entry := obj.(*v1alpha1.DNSEntry)
					return []string{ptr.Deref(entry.Status.Provider, "")}
				}).
			Build()

		count, err := CountEntriesForProvider(ctx, fakeClient, namespace, providerKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(int32(0)))
	})

	It("should count entries with provider set", func() {
		providerName := providerKey.String()
		entries := []client.Object{
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry1", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: ptr.To(providerName)},
			},
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry2", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: ptr.To(providerName)},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(entries...).
			WithIndex(&v1alpha1.DNSEntry{}, dnsprovider.EntryStatusProvider,
				func(obj client.Object) []string {
					entry := obj.(*v1alpha1.DNSEntry)
					return []string{ptr.Deref(entry.Status.Provider, "")}
				}).
			Build()

		count, err := CountEntriesForProvider(ctx, fakeClient, namespace, providerKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(int32(2)))
	})

	It("should only count entries with status.provider set", func() {
		providerName := providerKey.String()
		entries := []client.Object{
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry1", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: ptr.To(providerName)},
			},
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry2", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: nil},
			},
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry3", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(entries...).
			WithIndex(&v1alpha1.DNSEntry{}, dnsprovider.EntryStatusProvider,
				func(obj client.Object) []string {
					entry := obj.(*v1alpha1.DNSEntry)
					return []string{ptr.Deref(entry.Status.Provider, "")}
				}).
			Build()

		count, err := CountEntriesForProvider(ctx, fakeClient, namespace, providerKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(int32(1)))
	})

	It("should only count entries for the specified provider", func() {
		providerName := providerKey.String()
		entries := []client.Object{
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry1", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: ptr.To(providerName)},
			},
			&v1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{Name: "entry2", Namespace: namespace},
				Status:     v1alpha1.DNSEntryStatus{Provider: ptr.To("default/other-provider")},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(entries...).
			WithIndex(&v1alpha1.DNSEntry{}, dnsprovider.EntryStatusProvider,
				func(obj client.Object) []string {
					entry := obj.(*v1alpha1.DNSEntry)
					return []string{ptr.Deref(entry.Status.Provider, "")}
				}).
			Build()

		count, err := CountEntriesForProvider(ctx, fakeClient, namespace, providerKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(int32(1)))
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
