/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package integration

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var _ = Describe("SingleEntryOneProvider", func() {
	It("should deal with included and excluded domains", func() {
		baseDomain := "pr-1.inmemory.mock"
		pr, domain, err := testEnv.CreateSecretAndProvider(baseDomain, 0)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr)

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).Should(BeNil())

		checkProvider(pr)

		checkEntry(e, pr)

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Domains.Include = []string{"x." + baseDomain}
			spec.Domains.Exclude = []string{}
			return nil
		})
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryError(e.GetName())
		Ω(err).Should(BeNil())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Domains.Include = []string{domain}
			spec.Domains.Exclude = []string{}
			return nil
		})
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).Should(BeNil())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Domains.Include = []string{domain}
			spec.Domains.Exclude = []string{UnwrapEntry(e).Spec.DNSName}
			return nil
		})
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryStale(e.GetName())
		Ω(err).Should(BeNil())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Domains.Include = []string{domain}
			spec.Domains.Exclude = []string{}
			return nil
		})
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).Should(BeNil())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.ProviderConfig = testEnv.BuildProviderConfig(domain, baseDomain, FailGetZones)
			return nil
		})
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryStale(e.GetName())
		Ω(err).Should(BeNil())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.ProviderConfig = testEnv.BuildProviderConfig(domain, baseDomain)
			return nil
		})
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).Should(BeNil())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).Should(BeNil())

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).Should(BeNil())
	})
	It("should not delete entry if delete request fails", func() {
		baseDomain := "pr-1.inmemory.mock"
		pr, domain, err := testEnv.CreateSecretAndProvider(baseDomain, 0, FailDeleteEntry)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr)

		checkProvider(pr)

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).Should(BeNil())
		checkEntry(e, pr)

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).Should(BeNil())

		err = testEnv.MockInMemoryHasEntry(e)
		Ω(err).Should(BeNil())

		err = testEnv.DeleteEntryAndWait(e)
		if err == nil {
			Fail("delete must fail, as deleting mock DNS record has failed")
		}
		Ω(err.Error()).Should(ContainSubstring("Timeout"))
		err = testEnv.MockInMemoryHasEntry(e)
		Ω(err).Should(BeNil())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.ProviderConfig = testEnv.BuildProviderConfig(domain, baseDomain)
			return nil
		})
		Ω(err).Should(BeNil())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).Should(BeNil())

		err = testEnv.MockInMemoryHasNotEntry(e)
		Ω(err).Should(BeNil())

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).Should(BeNil())
	})
})
