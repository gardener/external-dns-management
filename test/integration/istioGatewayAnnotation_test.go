// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/api/networking/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("IstioGatewayAnnotation", func() {
	It("creates DNS entry for gateway backed by service", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())
		println(pr)
		defer testEnv.DeleteProviderAndSecret(pr)

		fakeExternalIP := "1.2.3.4"
		status := &v1.LoadBalancerIngress{IP: fakeExternalIP}
		svcDomain := "mysvc." + domain
		svcDomain2 := "mysvc2." + domain
		ttl := 456
		svc, gw, err := testEnv.CreateServiceAndIstioGatewayWithAnnotation("mygateway", svcDomain, status, ttl, nil, nil)
		Ω(err).ShouldNot(HaveOccurred())
		svc2, gw2, err := testEnv.CreateServiceAndIstioGatewayWithAnnotation("mygateway2", svcDomain2, status, ttl, nil,
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
		Ω(entry.Spec.TTL).ShouldNot(BeNil())
		Ω(*entry.Spec.TTL).Should(Equal(int64(ttl)))
		Ω(entry.Spec.ResolveTargetsToAddresses).To(BeNil())

		testEnv.AnnotateObject(gw, "dns.gardener.cloud/ignore", "true")
		testEnv.AwaitEntryState(entryObj.GetName(), "Ignored")
		testEnv.AnnotateObject(gw, "dns.gardener.cloud/ignore", "")
		testEnv.AwaitEntryState(entryObj.GetName(), "Ready")
		testEnv.AnnotateObject(gw, "dns.gardener.cloud/ignore", "reconcile")
		testEnv.AwaitEntryState(entryObj.GetName(), "Ignored")
		testEnv.AnnotateObject(gw, "dns.gardener.cloud/ignore", "")
		testEnv.AwaitEntryState(entryObj.GetName(), "Ready")
		testEnv.AnnotateObject(gw, "dns.gardener.cloud/ignore", "full")
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

		err = svc.Delete()
		Ω(err).ShouldNot(HaveOccurred())
		err = svc2.Delete()
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

	It("creates DNS entry for gateway backed by ingress", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())
		println(pr)
		defer testEnv.DeleteProviderAndSecret(pr)

		fakeExternalIP := "5.5.5.5"
		lbIngress := &v12.IngressLoadBalancerIngress{IP: fakeExternalIP}
		svcDomain := "myingress." + domain
		ttl := 456
		ingress, gw, err := testEnv.CreateIngressAndIstioGatewayWithAnnotation("mygateway2", svcDomain, lbIngress, ttl, nil)
		Ω(err).ShouldNot(HaveOccurred())

		entryObj, err := testEnv.AwaitObjectByOwner("Gateway", gw.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		checkEntry(entryObj, pr)
		entryObj, err = testEnv.GetEntry(entryObj.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		entry := UnwrapEntry(entryObj)
		Ω(entry.Spec.DNSName).Should(Equal(svcDomain))
		Ω(entry.Spec.Targets).Should(ConsistOf(fakeExternalIP))
		Ω(entry.Spec.TTL).ShouldNot(BeNil())
		Ω(*entry.Spec.TTL).Should(Equal(int64(ttl)))

		err = gw.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = ingress.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitServiceDeletion(gw.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryDeletion(entryObj.GetName())
		Ω(err).ShouldNot(HaveOccurred())
	})
})
