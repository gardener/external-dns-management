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
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr)

		checkProvider(pr)

		start := time.Now()
		entries := []resources.Object{}
		for i := 0; i < 3; i++ {
			e, err := testEnv.CreateEntry(i+1, domain)
			Ω(err).Should(BeNil())
			entries = append(entries, e)
			defer testEnv.DeleteEntryAndWait(entries[i])
		}
		maxDuration := 0 * time.Second
		for i := 0; i < 3; i++ {
			checkEntry(entries[i], pr)
			end := time.Now()
			d := end.Sub(start)
			start = end
			if d > maxDuration {
				maxDuration = d
			}
		}
		// rate is limited to one request per 15s
		Ω(maxDuration > 14*time.Second).Should(BeTrue(), fmt.Sprintf("max: %.1f > 14s", maxDuration.Seconds()))

		start = time.Now()
		err = testEnv.DeleteEntriesAndWait(entries...)
		Ω(err).Should(BeNil())
		deleteDuration := time.Now().Sub(start)
		// delete operations are not rate limited
		Ω(deleteDuration < 15*time.Second).Should(BeTrue(), fmt.Sprintf("deletion: %.1f < 15s", maxDuration.Seconds()))

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).Should(BeNil())
	})
})
