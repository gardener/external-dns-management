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

var _ = Describe("Single DNSEntry", func() {
	It("has correct life cycle", func() {
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
})
