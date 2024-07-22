// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"strings"

	"github.com/gardener/controller-manager-library/pkg/resources"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("GatewayAPIGatewayAnnotation", func() {
	It("creates DNS entry for gateway listener with hostname", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())
		println(pr)
		defer testEnv.DeleteProviderAndSecret(pr)

		fakeExternalIP := "1.2.3.4"
		status := &gatewayapisv1.GatewayStatusAddress{Value: fakeExternalIP}
		svcDomain := "mysvc." + domain
		svcDomain2 := "mysvc2." + domain
		ttl := 456
		gw, err := testEnv.CreateGatewayAPIGatewayWithAnnotation("mygateway", svcDomain, status, ttl, nil, nil)
		Ω(err).ShouldNot(HaveOccurred())
		gw2, err := testEnv.CreateGatewayAPIGatewayWithAnnotation("mygateway2", svcDomain2, status, ttl, nil,
			map[string]string{"dns.gardener.cloud/resolve-targets-to-addresses": "true"})
		Ω(err).ShouldNot(HaveOccurred())

		entryObj, err := testEnv.AwaitObjectByOwner("Gateway", gw.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		entryObj2, err := testEnv.AwaitObjectByOwner("Gateway", gw2.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		checkEntry(entryObj, pr)
		entryObj, err = testEnv.GetEntry(entryObj.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		entry := UnwrapEntry(entryObj)
		Ω(entry.Spec.DNSName).Should(Equal(svcDomain))
		Ω(entry.Spec.Targets).Should(ConsistOf(fakeExternalIP))
		Ω(entry.Spec.OwnerId).Should(BeNil())
		Ω(entry.Spec.TTL).ShouldNot(BeNil())
		Ω(*entry.Spec.TTL).Should(Equal(int64(ttl)))
		Ω(entry.Spec.ResolveTargetsToAddresses).To(BeNil())

		testEnv.AnnotateObject(gw, "dns.gardener.cloud/ignore", "true")
		testEnv.AwaitEntryState(entryObj.GetName(), "Ignored")
		testEnv.AnnotateObject(gw, "dns.gardener.cloud/ignore", "")
		testEnv.AwaitEntryState(entryObj.GetName(), "Ready")

		checkEntry(entryObj2, pr)
		entryObj2, err = testEnv.GetEntry(entryObj2.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		entry2 := UnwrapEntry(entryObj2)
		Ω(entry2.Spec.DNSName).Should(Equal(svcDomain2))
		Ω(entry2.Spec.ResolveTargetsToAddresses).To(Equal(ptr.To(true)))

		err = gw.Delete()
		Ω(err).ShouldNot(HaveOccurred())
		err = gw2.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitServiceDeletion(gw.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitServiceDeletion(gw2.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryDeletion(entryObj.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitEntryDeletion(entryObj2.GetName())
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("creates DNS entries for gateway with httproutes with hostnames", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())
		println(pr)
		defer testEnv.DeleteProviderAndSecret(pr)

		fakeExternalIP := "1.2.3.4"
		status := &gatewayapisv1.GatewayStatusAddress{Value: fakeExternalIP}
		baseDomain := ".mysvc." + domain
		ttl := 456
		gw, err := testEnv.CreateGatewayAPIGatewayWithAnnotation("mygateway2", "", status, ttl, nil, nil)
		Ω(err).ShouldNot(HaveOccurred())

		route1, err := testEnv.CreateGatewayAPIHTTPRoute("route1", "route1"+baseDomain, gw.ObjectName())
		Ω(err).ShouldNot(HaveOccurred())

		route2, err := testEnv.CreateGatewayAPIHTTPRoute("route2", "route2"+baseDomain, gw.ObjectName())
		Ω(err).ShouldNot(HaveOccurred())
		route2b, err := testEnv.CreateGatewayAPIHTTPRoute("route2b", "route2"+baseDomain, gw.ObjectName())
		Ω(err).ShouldNot(HaveOccurred())

		entryObjs, err := testEnv.AwaitObjectsByOwner("Gateway", gw.GetName(), 2)
		Ω(err).ShouldNot(HaveOccurred())

		var entry1, entry2 resources.Object
		for _, entryObj := range entryObjs {
			checkEntry(entryObj, pr)
			entryObj, err = testEnv.GetEntry(entryObj.GetName())
			Ω(err).ShouldNot(HaveOccurred())
			entry := UnwrapEntry(entryObj)
			switch strings.TrimSuffix(entry.Spec.DNSName, baseDomain) {
			case "route1":
				entry1 = entryObj
			case "route2":
				entry2 = entryObj
			default:
				Fail("unexpected domain name: " + entry.Spec.DNSName)
			}
			Ω(entry.Spec.Targets).Should(ConsistOf(fakeExternalIP))
			Ω(entry.Spec.OwnerId).Should(BeNil())
			Ω(entry.Spec.TTL).ShouldNot(BeNil())
			Ω(*entry.Spec.TTL).Should(Equal(int64(ttl)))
		}

		err = route1.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = route2b.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryDeletion(entry1.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		entryObj, err := testEnv.AwaitObjectByOwner("Gateway", gw.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		Ω(entryObj.GetName()).Should(Equal(entry2.GetName()))

		err = gw.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = route2.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitServiceDeletion(gw.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryDeletion(entryObj.GetName())
		Ω(err).ShouldNot(HaveOccurred())
	})
})
