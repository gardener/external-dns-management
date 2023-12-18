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
	v1 "k8s.io/api/core/v1"
)

var _ = Describe("ServiceAnnotation", func() {
	It("creates DNS entry", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())
		println(pr)
		defer testEnv.DeleteProviderAndSecret(pr)

		fakeExternalIP := "1.2.3.4"
		status := &v1.LoadBalancerIngress{IP: fakeExternalIP}
		svcDomain := "mysvc." + domain
		ttl := 456
		svc, err := testEnv.CreateServiceWithAnnotation("mysvc", svcDomain, status, ttl, nil, nil)
		Ω(err).ShouldNot(HaveOccurred())
		routingPolicy := `{"type": "weighted", "setIdentifier": "my-id", "parameters": {"weight": "10"}}`
		svcDomain2 := "mysvc2." + domain
		svc2, err := testEnv.CreateServiceWithAnnotation("mysvc2", svcDomain2, status, ttl, &routingPolicy, nil)
		Ω(err).ShouldNot(HaveOccurred())

		// openstack proxy support
		svcDomain3 := "mysvc3." + domain
		annotations := map[string]string{
			"loadbalancer.openstack.org/hostname":              svcDomain3,
			"loadbalancer.openstack.org/load-balancer-address": fakeExternalIP,
		}
		status3 := &v1.LoadBalancerIngress{Hostname: svcDomain3}
		svc3, err := testEnv.CreateServiceWithAnnotation("mysvc3", svcDomain3, status3, ttl, nil, annotations)
		Ω(err).ShouldNot(HaveOccurred())

		svcDomain4 := "mysvc4." + domain
		svc4, err := testEnv.CreateServiceWithAnnotation("mysvc4", svcDomain4, status, ttl, &routingPolicy,
			map[string]string{
				"dns.gardener.cloud/owner-id": "second",
			})
		Ω(err).ShouldNot(HaveOccurred())

		entryObj, err := testEnv.AwaitObjectByOwner("Service", svc.GetName())
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

		entryObj2, err := testEnv.AwaitObjectByOwner("Service", svc2.GetName())
		entry2 := UnwrapEntry(entryObj2)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(entry2.Spec.DNSName).Should(Equal(svcDomain2))
		Ω(entry2.Spec.RoutingPolicy).ShouldNot(BeNil())
		Ω(*entry2.Spec.RoutingPolicy).Should(Equal(v1alpha1.RoutingPolicy{
			Type:          "weighted",
			SetIdentifier: "my-id",
			Parameters:    map[string]string{"weight": "10"},
		}))

		entryObj3, err := testEnv.AwaitObjectByOwner("Service", svc3.GetName())
		entry3 := UnwrapEntry(entryObj3)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(entry3.Spec.DNSName).Should(Equal(svcDomain3))
		Ω(entry3.Spec.Targets).Should(ConsistOf(fakeExternalIP))

		entryObj4, err := testEnv.AwaitObjectByOwner("Service", svc4.GetName())
		entry4 := UnwrapEntry(entryObj4)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(entry4.Spec.DNSName).Should(Equal(svcDomain4))
		Ω(entry4.Spec.DNSName).Should(Equal(svcDomain4))
		Ω(entry4.Spec.OwnerId).ShouldNot(BeNil())
		Ω(*entry4.Spec.OwnerId).Should(Equal("second"))

		err = svc.Delete()
		Ω(err).ShouldNot(HaveOccurred())
		err = svc2.Delete()
		Ω(err).ShouldNot(HaveOccurred())
		err = svc3.Delete()
		Ω(err).ShouldNot(HaveOccurred())
		err = svc4.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitServiceDeletion(svc.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitServiceDeletion(svc2.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitServiceDeletion(svc3.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitServiceDeletion(svc4.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryDeletion(entryObj.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitEntryDeletion(entryObj2.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitEntryDeletion(entryObj3.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitEntryDeletion(entryObj4.GetName())
		Ω(err).ShouldNot(HaveOccurred())
	})
})
