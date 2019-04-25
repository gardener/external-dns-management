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
)

func createDelete() {
	secretName := testEnv.SecretName(0)
	pr, _, err := testEnv.CreateProvider("inmemory.mock", 0, secretName)
	Ω(err).Should(BeNil())
	defer testEnv.DeleteProviderAndSecret(pr)

	checkHasFinalizer(pr)

	err = testEnv.AwaitProviderState(pr.GetName(), "Error")
	Ω(err).Should(BeNil())

	// create secret after provider
	secret, err := testEnv.CreateSecret(0)
	Ω(err).Should(BeNil())

	// provider should be ready now
	checkProvider(pr)

	checkHasFinalizer(secret)
}

var _ = Describe("Provider_Secret", func() {
	It("works if secret is created after provider", func() {
		Context("first round", createDelete)

		secretName := testEnv.SecretName(0)
		err := testEnv.AwaitSecretDeletion(secretName)
		Ω(err).Should(BeNil())

		Context("second round", createDelete)
	})
})
