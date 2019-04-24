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
	. "github.com/onsi/gomega"
)

func checkHasFinalizer(obj resources.Object) {
	err := testEnv.AwaitFinalizers(obj, "dns.gardener.cloud/mock-inmemory")
	Ω(err).ShouldNot(HaveOccurred())
}

func checkProvider(obj resources.Object) {
	err := testEnv.AwaitProviderReady(obj.GetName())
	Ω(err).Should(BeNil())

	checkHasFinalizer(obj)
}

func checkEntry(obj resources.Object, provider resources.Object) {
	err := testEnv.AwaitEntryReady(obj.GetName())
	Ω(err).Should(BeNil())

	checkHasFinalizer(obj)

	_, entry, err := testEnv.GetEntry(obj.GetName())
	Ω(err).Should(BeNil())
	Ω(entry.Status.ProviderType).ShouldNot(BeNil(), "Missing provider type")
	Ω(*entry.Status.ProviderType).Should(Equal("mock-inmemory"))
	Ω(entry.Status.Provider).ShouldNot(BeNil(), "Missing provider")
	providerName := provider.ObjectName().String()
	Ω(*entry.Status.Provider).Should(Equal(providerName))
}
