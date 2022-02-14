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
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	. "github.com/onsi/gomega"
)

func checkHasFinalizer(obj resources.Object) {
	checkHasFinalizerEx(testEnv, obj)
}

func checkHasFinalizerEx(te *TestEnv, obj resources.Object) {
	err := te.AwaitFinalizers(obj, "dns.gardener.cloud/compound")
	Ω(err).ShouldNot(HaveOccurred())
}

func checkProvider(obj resources.Object) {
	checkProviderEx(testEnv, obj)
}

func checkProviderEx(te *TestEnv, obj resources.Object) {
	err := testEnv.AwaitProviderReady(obj.GetName())
	Ω(err).Should(BeNil())

	checkHasFinalizer(obj)
}

func checkEntry(obj resources.Object, provider resources.Object) *v1alpha1.DNSEntry {
	return checkEntryEx(testEnv, obj, provider)
}

func checkEntryEx(te *TestEnv, obj resources.Object, provider resources.Object, providerType ...string) *v1alpha1.DNSEntry {
	err := te.AwaitEntryReady(obj.GetName())
	Ω(err).Should(BeNil())

	checkHasFinalizerEx(te, obj)

	entryObj, err := te.GetEntry(obj.GetName())
	Ω(err).Should(BeNil())
	entry := UnwrapEntry(entryObj)
	Ω(entry.Status.ProviderType).ShouldNot(BeNil(), "Missing provider type")
	typ := "mock-inmemory"
	if len(providerType) == 1 {
		typ = providerType[0]
	}
	Ω(*entry.Status.ProviderType).Should(Equal(typ))
	Ω(entry.Status.Provider).ShouldNot(BeNil(), "Missing provider")
	providerName := provider.ObjectName().String()
	Ω(*entry.Status.Provider).Should(Equal(providerName))
	return entry
}
