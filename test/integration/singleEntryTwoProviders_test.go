// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SingleEntryTwoProviders", func() {
	It("has correct life cycle", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("pr-1.inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr)

		pr2, _, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 1)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr2)

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).ShouldNot(HaveOccurred())

		checkProvider(pr)

		checkEntry(e, pr)

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).ShouldNot(HaveOccurred())

		// should be moved to other provider pr2
		checkEntry(e, pr2)

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteProviderAndSecret(pr2)
		Ω(err).ShouldNot(HaveOccurred())
	})
})
