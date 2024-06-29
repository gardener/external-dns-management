// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
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
		svc, err := testEnv.CreateServiceWithAnnotation("mysvc", svcDomain, status, ttl, nil,
			map[string]string{"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "dualstack"})
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

		svcDomain5 := "mysvc5." + domain
		svc5, err := testEnv.CreateServiceWithAnnotation("mysvc5", svcDomain5, status, ttl, nil,
			map[string]string{
				"dns.gardener.cloud/resolve-targets-to-addresses": "true",
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
		Ω(entry.Annotations["dns.gardener.cloud/ip-stack"]).Should(Equal("dual-stack"))

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
		Ω(entry4.Spec.OwnerId).ShouldNot(BeNil())
		Ω(*entry4.Spec.OwnerId).Should(Equal("second"))

		entryObj5, err := testEnv.AwaitObjectByOwner("Service", svc5.GetName())
		entry5 := UnwrapEntry(entryObj5)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(entry5.Spec.DNSName).Should(Equal(svcDomain5))
		Ω(entry5.Spec.ResolveTargetsToAddresses).To(Equal(ptr.To(true)))

		for _, item := range []resources.Object{svc, svc2, svc3, svc4, svc5} {
			Ω(item.Delete()).ShouldNot(HaveOccurred())
		}

		for _, item := range []resources.Object{svc, svc2, svc3, svc4, svc5} {
			Ω(testEnv.AwaitServiceDeletion(item.GetName())).ShouldNot(HaveOccurred())
		}

		for _, item := range []resources.Object{entryObj, entryObj2, entryObj3, entryObj4, entryObj5} {
			Ω(testEnv.AwaitEntryDeletion(item.GetName())).ShouldNot(HaveOccurred())
		}
	})

	It("creates DNS entries for DNSAnnotations", func() {
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

		entryObj, err := testEnv.AwaitObjectByOwner("Service", svc.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		checkEntry(entryObj, pr)
		entryObj, err = testEnv.GetEntry(entryObj.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		annot1, err := testEnv.CreateDNSAnnotationForService("annot1", v1alpha1.DNSAnnotationSpec{
			ResourceRef: v1alpha1.ResourceReference{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       svc.GetName(),
				Namespace:  svc.GetNamespace(),
			},
			Annotations: map[string]string{
				"dns.gardener.cloud/dnsnames": "test1.foo.bar",
			},
		})
		Ω(err).ShouldNot(HaveOccurred())
		annot2, err := testEnv.CreateDNSAnnotationForService("annot2", v1alpha1.DNSAnnotationSpec{
			ResourceRef: v1alpha1.ResourceReference{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       svc.GetName(),
				Namespace:  svc.GetNamespace(),
			},
			Annotations: map[string]string{
				"dns.gardener.cloud/dnsnames": "test2.foo.bar",
			},
		})
		Ω(err).ShouldNot(HaveOccurred())

		entries, err := testEnv.AwaitObjectsByOwner("Service", svc.GetName(), 3)
		Ω(err).ShouldNot(HaveOccurred())
		found := 0
		for _, e := range entries {
			name := e.Data().(*v1alpha1.DNSEntry).Spec.DNSName
			if name == "test1.foo.bar" {
				found += 1
			}
			if name == "test2.foo.bar" {
				found += 2
			}
		}
		Ω(found).Should(Equal(3))

		err = annot2.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		_, err = testEnv.AwaitObjectsByOwner("Service", svc.GetName(), 2)
		Ω(err).ShouldNot(HaveOccurred())

		err = annot1.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		_, err = testEnv.AwaitObjectByOwner("Service", svc.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = svc.Delete()
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryDeletion(entryObj.GetName())
		Ω(err).ShouldNot(HaveOccurred())
	})
})
