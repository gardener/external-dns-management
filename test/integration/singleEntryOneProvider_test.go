// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var _ = Describe("SingleEntryOneProvider", func() {
	It("should deal with included and excluded domains", func() {
		pr, domain, domain2, err := testEnv.CreateSecretAndProvider("pr-1.inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr)

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).ShouldNot(HaveOccurred())

		checkProvider(pr)

		checkEntry(e, pr)

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Domains.Include = []string{"x." + domain2}
			spec.Domains.Exclude = []string{}
			return nil
		})
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryError(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Domains.Include = []string{domain}
			spec.Domains.Exclude = []string{}
			return nil
		})
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Domains.Include = []string{domain}
			spec.Domains.Exclude = []string{UnwrapEntry(e).Spec.DNSName}
			return nil
		})
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryStale(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Domains.Include = []string{domain}
			spec.Domains.Exclude = []string{}
			return nil
		})
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.ProviderConfig = testEnv.BuildProviderConfig(domain, domain2, FailGetZones)
			return nil
		})
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryStale(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.ProviderConfig = testEnv.BuildProviderConfig(domain, domain2)
			return nil
		})
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).ShouldNot(HaveOccurred())
	})
	It("should not delete entry if delete request fails", func() {
		pr, domain, domain2, err := testEnv.CreateSecretAndProvider("pr-1.inmemory.mock", 0, FailDeleteEntry)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr)

		checkProvider(pr)

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).ShouldNot(HaveOccurred())
		checkEntry(e, pr)

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.MockInMemoryHasEntry(e)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteEntryAndWait(e)
		if err == nil {
			Fail("delete must fail, as deleting mock DNS record has failed")
		}
		Ω(err.Error()).Should(ContainSubstring("timeout during check"))
		err = testEnv.MockInMemoryHasEntry(e)
		Ω(err).ShouldNot(HaveOccurred())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.ProviderConfig = testEnv.BuildProviderConfig(domain, domain2)
			return nil
		})
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.MockInMemoryHasNotEntry(e)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).ShouldNot(HaveOccurred())
	})
})
