// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"math"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	count = 50
	half  = count / 2
)

var _ = Describe("ManyEntriesManyProviders", func() {
	It("has correct lifecycle", func() {
		oldTimeout := testEnv.defaultTimeout
		testEnv.defaultTimeout = oldTimeout * time.Duration(int64(math.Sqrt(entryCount)))
		defer func() { testEnv.defaultTimeout = oldTimeout }()

		providers := []resources.Object{}
		entries := []resources.Object{}
		for i := 0; i < count; i++ {
			pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", i)
			Ω(err).ShouldNot(HaveOccurred())
			// defer testEnv.DeleteProviderAndSecret(pr)
			providers = append(providers, pr)

			entry, err := testEnv.CreateEntry(i, domain)
			Ω(err).ShouldNot(HaveOccurred())
			entries = append(entries, entry)
		}

		for _, provider := range providers {
			checkProvider(provider)
		}

		for i, entry := range entries {
			checkEntry(entry, providers[i])
		}

		for _, entry := range entries[:half] {
			err := testEnv.DeleteEntryAndWait(entry)
			Ω(err).ShouldNot(HaveOccurred())
		}

		for _, provider := range providers {
			err := testEnv.DeleteProviderAndSecret(provider)
			Ω(err).ShouldNot(HaveOccurred())
		}

		for _, entry := range entries[half:] {
			err := testEnv.AwaitEntryStale(entry.GetName())
			Ω(err).ShouldNot(HaveOccurred())

			err = testEnv.AwaitFinalizers(entry)
			Ω(err).ShouldNot(HaveOccurred())

			err = entry.Delete()
			Ω(err).ShouldNot(HaveOccurred())
		}

		for _, entry := range entries[half:] {
			err := testEnv.AwaitEntryDeletion(entry.GetName())
			Ω(err).ShouldNot(HaveOccurred())
		}
	})
})
