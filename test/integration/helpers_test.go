// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"github.com/gardener/controller-manager-library/pkg/resources"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
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
	Ω(err).ShouldNot(HaveOccurred())

	checkHasFinalizerEx(te, obj)
}

func checkEntry(obj resources.Object, provider resources.Object) *v1alpha1.DNSEntry {
	return checkEntryEx(testEnv, obj, provider)
}

func checkEntryEx(te *TestEnv, obj resources.Object, provider resources.Object, providerType ...string) *v1alpha1.DNSEntry {
	err := te.AwaitEntryReady(obj.GetName())
	Ω(err).ShouldNot(HaveOccurred())

	checkHasFinalizerEx(te, obj)

	entryObj, err := te.GetEntry(obj.GetName())
	Ω(err).ShouldNot(HaveOccurred())
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
	Ω(entry.Status.DNSName).Should(Equal(ptr.To(dns.NormalizeHostname(entry.Spec.DNSName))))
	return entry
}
