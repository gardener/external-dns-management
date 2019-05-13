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
	"math"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const entryCount = 50

var _ = Describe("ManyEntriesOneProvider", func() {
	It("has correct lifecycle", func() {
		oldTimeout := testEnv.defaultTimeout
		testEnv.defaultTimeout = oldTimeout * time.Duration(int64(math.Sqrt(entryCount)))
		defer func() { testEnv.defaultTimeout = oldTimeout }()

		pr, domain, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr)

		entries := []resources.Object{}
		for i := 0; i < entryCount; i++ {
			e, err := testEnv.CreateEntry(i, domain)
			Ω(err).Should(BeNil())
			entries = append(entries, e)
		}

		checkProvider(pr)

		for _, entry := range entries {
			checkEntry(entry, pr)
		}

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).Should(BeNil())

		for _, entry := range entries {
			err = testEnv.AwaitEntryState(entry.GetName(), "Error", "")
			Ω(err).Should(BeNil())

			err = testEnv.AwaitFinalizers(entry)
			Ω(err).Should(BeNil())

			err = entry.Delete()
			Ω(err).Should(BeNil())
		}

		for _, entry := range entries {
			err = testEnv.AwaitEntryDeletion(entry.GetName())
			Ω(err).Should(BeNil())
		}
	})
})
