// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/api/networking/v1"
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
		ttl := 456
		svc, gw, err := testEnv.CreateServiceAndIstioGatewayWithAnnotation("mygateway", svcDomain, status, ttl, nil, nil)
		Ω(err).ShouldNot(HaveOccurred())

		entryObj, err := testEnv.AwaitObjectByOwner("Gateway", gw.GetName())
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

		err = gw.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = svc.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitServiceDeletion(gw.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryDeletion(entryObj.GetName())
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
		Ω(entry.Spec.OwnerId).Should(BeNil())
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
