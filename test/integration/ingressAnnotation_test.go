// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var _ = Describe("IngressAnnotation", func() {
	It("creates DNS entry", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())
		println(pr)
		defer testEnv.DeleteProviderAndSecret(pr)

		fakeExternalIP := "1.2.3.4"
		ingressDomain := "myingress." + domain
		ttl := 456
		ingress, err := testEnv.CreateIngressWithAnnotation("myingress", ingressDomain, fakeExternalIP, ttl, nil,
			map[string]string{"dns.gardener.cloud/ip-stack": "dual-stack"})
		Ω(err).ShouldNot(HaveOccurred())
		routingPolicy := `{"type": "weighted", "setIdentifier": "my-id", "parameters": {"weight": "10"}}`
		ingress2, err := testEnv.CreateIngressWithAnnotation("myingress2", ingressDomain, fakeExternalIP, ttl, &routingPolicy, nil)
		Ω(err).ShouldNot(HaveOccurred())
		ingress3, err := testEnv.CreateIngressWithAnnotation("myingress3", ingressDomain, fakeExternalIP, ttl, nil,
			map[string]string{"dns.gardener.cloud/resolve-targets-to-addresses": "true"})
		Ω(err).ShouldNot(HaveOccurred())

		entryObj, err := testEnv.AwaitObjectByOwner("Ingress", ingress.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		checkEntry(entryObj, pr)
		entryObj, err = testEnv.GetEntry(entryObj.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		entry := UnwrapEntry(entryObj)
		Ω(entry.Spec.DNSName).Should(Equal(ingressDomain))
		Ω(entry.Spec.Targets).Should(ConsistOf(fakeExternalIP))
		Ω(entry.Spec.TTL).ShouldNot(BeNil())
		Ω(*entry.Spec.TTL).Should(Equal(int64(ttl)))
		Ω(entry.Annotations["dns.gardener.cloud/ip-stack"]).Should(Equal("dual-stack"))

		testEnv.AnnotateObject(ingress, "dns.gardener.cloud/ignore", "true")
		testEnv.AwaitEntryState(entryObj.GetName(), "Ignored")
		testEnv.AnnotateObject(ingress, "dns.gardener.cloud/ignore", "")
		testEnv.AwaitEntryState(entryObj.GetName(), "Ready")

		// keep old targets if ingress lost its load balancers
		err = testEnv.PatchIngressLoadBalancer(ingress, "")
		Ω(err).ShouldNot(HaveOccurred())

		entryObj2, err := testEnv.AwaitObjectByOwner("Ingress", ingress2.GetName())
		entry2 := UnwrapEntry(entryObj2)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(entry2.Spec.RoutingPolicy).ShouldNot(BeNil())
		Ω(*entry2.Spec.RoutingPolicy).Should(Equal(v1alpha1.RoutingPolicy{
			Type:          "weighted",
			SetIdentifier: "my-id",
			Parameters:    map[string]string{"weight": "10"},
		}))
		Ω(entry2.Annotations["dns.gardener.cloud/ip-stack"]).Should(Equal(""))
		Ω(entry2.Spec.ResolveTargetsToAddresses).To(BeNil())

		entryObj3, err := testEnv.AwaitObjectByOwner("Ingress", ingress3.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		entry3 := UnwrapEntry(entryObj3)
		Ω(entry3.Spec.ResolveTargetsToAddresses).To(Equal(ptr.To(true)))

		err = ingress2.Delete()
		Ω(err).ShouldNot(HaveOccurred())
		err = ingress3.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitIngressDeletion(ingress2.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitIngressDeletion(ingress3.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryDeletion(entryObj2.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitEntryDeletion(entryObj3.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		// check unchanged target
		entryObj, err = testEnv.AwaitObjectByOwner("Ingress", ingress.GetName())
		entry = UnwrapEntry(entryObj)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(entry.Spec.Targets).Should(ConsistOf(fakeExternalIP))

		err = ingress.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitIngressDeletion(ingress.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryDeletion(entryObj.GetName())
		Ω(err).ShouldNot(HaveOccurred())
	})
})
