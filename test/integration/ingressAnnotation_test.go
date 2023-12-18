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
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		ingress, err := testEnv.CreateIngressWithAnnotation("myingress", ingressDomain, fakeExternalIP, ttl, nil, nil)
		Ω(err).ShouldNot(HaveOccurred())
		routingPolicy := `{"type": "weighted", "setIdentifier": "my-id", "parameters": {"weight": "10"}}`
		ingress2, err := testEnv.CreateIngressWithAnnotation("mysvc2", ingressDomain, fakeExternalIP, ttl, &routingPolicy, nil)
		Ω(err).ShouldNot(HaveOccurred())
		ingress3, err := testEnv.CreateIngressWithAnnotation("mysvc3", ingressDomain, fakeExternalIP, ttl, nil,
			map[string]string{"dns.gardener.cloud/owner-id": "second"})
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
		Ω(entry.Spec.OwnerId).Should(BeNil())

		entryObj2, err := testEnv.AwaitObjectByOwner("Ingress", ingress2.GetName())
		entry2 := UnwrapEntry(entryObj2)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(entry2.Spec.RoutingPolicy).ShouldNot(BeNil())
		Ω(*entry2.Spec.RoutingPolicy).Should(Equal(v1alpha1.RoutingPolicy{
			Type:          "weighted",
			SetIdentifier: "my-id",
			Parameters:    map[string]string{"weight": "10"},
		}))

		entryObj3, err := testEnv.AwaitObjectByOwner("Ingress", ingress3.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		entry3 := UnwrapEntry(entryObj3)
		Ω(entry3.Spec.OwnerId).ShouldNot(BeNil())
		Ω(*entry3.Spec.OwnerId).Should(Equal("second"))

		err = ingress.Delete()
		Ω(err).ShouldNot(HaveOccurred())
		err = ingress2.Delete()
		Ω(err).ShouldNot(HaveOccurred())
		err = ingress3.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitIngressDeletion(ingress.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitIngressDeletion(ingress2.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitIngressDeletion(ingress3.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryDeletion(entryObj.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitEntryDeletion(entryObj2.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitEntryDeletion(entryObj3.GetName())
		Ω(err).ShouldNot(HaveOccurred())
	})
})
