// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ProviderRateLimits", func() {
	It("should respect provider rate limits", func() {
		// need longer timeout because in worst case: 10s (batch) + 15s (delay zone) = 25s
		defaultTimeout := testEnv.defaultTimeout
		testEnv.defaultTimeout = 45 * time.Second
		defer func() { testEnv.defaultTimeout = defaultTimeout }()

		pr, domain, _, err := testEnv.CreateSecretAndProvider("pr-1.inmemory.mock", 0, Quotas4PerMin)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr)

		checkProvider(pr)

		start := time.Now()
		entries := []resources.Object{}
		for i := range 3 {
			e, err := testEnv.CreateEntry(i+1, domain)
			Ω(err).ShouldNot(HaveOccurred())
			entries = append(entries, e)
			defer testEnv.DeleteEntryAndWait(entries[i])
		}
		maxDuration := 0 * time.Second
		for i := range 3 {
			checkEntry(entries[i], pr)
			end := time.Now()
			d := end.Sub(start)
			start = end
			if d > maxDuration {
				maxDuration = d
			}
		}
		// rate is limited to one request per 15s
		Ω(maxDuration).Should(BeNumerically(">", 14*time.Second), fmt.Sprintf("max: %.1f > 14s", maxDuration.Seconds()))

		start = time.Now()
		err = testEnv.DeleteEntriesAndWait(entries...)
		Ω(err).ShouldNot(HaveOccurred())
		deleteDuration := time.Since(start)
		// delete operations are not rate limited
		Ω(deleteDuration).Should(BeNumerically("<", 15*time.Second), fmt.Sprintf("deletion: %.1f < 15s", maxDuration.Seconds()))

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).ShouldNot(HaveOccurred())
	})
})
