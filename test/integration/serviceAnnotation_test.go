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
	"github.com/gardener/controller-manager-library/pkg/resources"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceAnnotation", func() {
	It("creates DNS entry", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).Should(BeNil())
		println(pr)
		defer testEnv.DeleteProviderAndSecret(pr)

		fakeExternalIP := "1.2.3.4"
		svcDomain := "mysvc." + domain
		ttl := 456
		svc, err := testEnv.CreateServiceWithAnnotation("mysvc", svcDomain, fakeExternalIP, ttl)
		Ω(err).Should(BeNil())

		var entryObj resources.Object
		err = testEnv.Await("Generated entry for service not found", func() (bool, error) {
			var err error
			entryObj, err = testEnv.FindEntryByOwner("Service", svc.GetName())
			if entryObj != nil {
				return true, nil
			}
			return false, err
		})
		Ω(err).Should(BeNil())

		checkEntry(entryObj, pr)

		entryObj, err = testEnv.GetEntry(entryObj.GetName())
		Ω(err).Should(BeNil())
		entry := UnwrapEntry(entryObj)
		Ω(entry.Spec.DNSName).Should(Equal(svcDomain))
		Ω(entry.Spec.Targets).Should(ConsistOf(fakeExternalIP))
		Ω(entry.Spec.TTL).ShouldNot(BeNil())
		Ω(*entry.Spec.TTL).Should(Equal(int64(ttl)))

		err = svc.Delete()
		Ω(err).Should(BeNil())

		err = testEnv.AwaitServiceDeletion(svc.GetName())
		Ω(err).Should(BeNil())

		err = testEnv.AwaitEntryDeletion(entryObj.GetName())
		Ω(err).Should(BeNil())
	})
})
