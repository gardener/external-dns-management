// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("QuotaExceededEntriesMap", func() {
	var qmap *quotaExceededEntriesMap

	BeforeEach(func() {
		qmap = newQuotaExceededEntriesMap()
	})

	Context("Add and Remove operations", func() {
		It("should add and retrieve entries", func() {
			entryKey1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			qmap.Add(entryKey1, providerKey)
			Expect(qmap.GetProvider(entryKey1)).To(Equal(&providerKey))
		})

		It("should remove entries", func() {
			entryKey1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			qmap.Add(entryKey1, providerKey)
			qmap.Remove(entryKey1)
			Expect(qmap.GetProvider(entryKey1)).To(BeNil())
		})

		It("should handle multiple entries for same provider", func() {
			entryKey1 := client.ObjectKey{Namespace: "default", Name: "entry1"}
			entryKey2 := client.ObjectKey{Namespace: "default", Name: "entry2"}
			providerKey := client.ObjectKey{Namespace: "default", Name: "provider1"}

			qmap.Add(entryKey1, providerKey)
			qmap.Add(entryKey2, providerKey)

			Expect(qmap.GetProvider(entryKey1)).To(Equal(&providerKey))
			Expect(qmap.GetProvider(entryKey2)).To(Equal(&providerKey))
		})
	})

	Context("GetProvider for non-existent entry", func() {
		It("should return nil", func() {
			entryKey := client.ObjectKey{Namespace: "default", Name: "entry1"}
			Expect(qmap.GetProvider(entryKey)).To(BeNil())
		})
	})
})
