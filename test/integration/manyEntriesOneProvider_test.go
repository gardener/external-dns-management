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

const entryCount = 50

var _ = Describe("ManyEntriesOneProvider", func() {
	It("has correct lifecycle", func() {
		oldTimeout := testEnv.defaultTimeout
		testEnv.defaultTimeout = oldTimeout * time.Duration(int64(math.Sqrt(entryCount)))
		defer func() { testEnv.defaultTimeout = oldTimeout }()

		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr)

		entries := []resources.Object{}
		for i := range entryCount {
			e, err := testEnv.CreateEntry(i, domain)
			Ω(err).ShouldNot(HaveOccurred())
			entries = append(entries, e)
		}

		checkProvider(pr)

		for _, entry := range entries {
			checkEntry(entry, pr)
		}

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).ShouldNot(HaveOccurred())

		for _, entry := range entries {
			err = testEnv.AwaitEntryStale(entry.GetName())
			Ω(err).ShouldNot(HaveOccurred())

			err = testEnv.AwaitFinalizers(entry)
			Ω(err).ShouldNot(HaveOccurred())

			err = entry.Delete()
			Ω(err).ShouldNot(HaveOccurred())
		}

		for _, entry := range entries {
			err = testEnv.AwaitEntryDeletion(entry.GetName())
			Ω(err).ShouldNot(HaveOccurred())
		}
	})
})
