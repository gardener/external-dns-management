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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("EntryLivecycle", func() {
	It("has correct life cycle with provider", func() {
		pr, domain, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).Should(BeNil())

		defer testEnv.DeleteProviderAndSecret(pr)

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).Should(BeNil())

		checkProvider(pr)

		checkEntry(e, pr)

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryState(e.GetName(), "Error", "")
		Ω(err).Should(BeNil())

		time.Sleep(10 * time.Second)

		err = testEnv.AwaitEntryState(e.GetName(), "Error")
		Ω(err).Should(BeNil())

		err = testEnv.AwaitFinalizers(e)
		Ω(err).Should(BeNil())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).Should(BeNil())
	})

	It("has correct life cycle with provider for TXT record", func() {
		pr, domain, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).Should(BeNil())

		defer testEnv.DeleteProviderAndSecret(pr)

		e, err := testEnv.CreateTXTEntry(0, domain)
		Ω(err).Should(BeNil())

		checkProvider(pr)

		checkEntry(e, pr)

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryState(e.GetName(), "Error", "")
		Ω(err).Should(BeNil())

		time.Sleep(10 * time.Second)

		err = testEnv.AwaitEntryState(e.GetName(), "Error")
		Ω(err).Should(BeNil())

		err = testEnv.AwaitFinalizers(e)
		Ω(err).Should(BeNil())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).Should(BeNil())
	})

	It("is handled only by owner", func() {
		pr, domain, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).Should(BeNil())

		defer testEnv.DeleteProviderAndSecret(pr)

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteEntryAndWait(e)

		checkProvider(pr)

		checkEntry(e, pr)

		ownerID := "my/owner1"
		e, err = testEnv.UpdateEntryOwner(e, &ownerID)
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryState(e.GetName(), "Stale")
		Ω(err).Should(BeNil())

		owner1, err := testEnv.CreateOwner("owner1", ownerID)
		Ω(err).Should(BeNil())

		defer owner1.Delete()

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).Should(BeNil())

		ownerID2 := "my/owner2"
		e, err = testEnv.UpdateEntryOwner(e, &ownerID2)
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryState(e.GetName(), "Stale")
		Ω(err).Should(BeNil())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).Should(BeNil())
	})

	It("is handled only by matching provider", func() {
		pr, domain, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).Should(BeNil())

		defer testEnv.DeleteProviderAndSecret(pr)

		e, err := testEnv.CreateEntry(0, domain)
		dnsName := UnwrapEntry(e).Spec.DNSName
		Ω(err).Should(BeNil())
		defer testEnv.DeleteEntryAndWait(e)

		checkProvider(pr)

		checkEntry(e, pr)

		e, err = testEnv.UpdateEntryDomain(e, "foo.mock")
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryState(e.GetName(), "Error")
		Ω(err).Should(BeNil())

		e, err = testEnv.UpdateEntryDomain(e, dnsName)
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).Should(BeNil())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).Should(BeNil())
	})
})
