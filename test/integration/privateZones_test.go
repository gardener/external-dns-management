// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/provider/local"
)

var _ = Describe("PrivateZones", func() {
	It("should deal with providers in different namespaces, different zones, but same base domain", func() {
		baseDomain := "pr-1.inmemory.mock"

		var err error
		envs := []*TestEnv{testEnv, testEnv2}
		var providers []resources.Object
		var entries []resources.Object
		var domains []string
		for _, te := range envs {
			pr, domain, _, err := te.CreateSecretAndProvider(baseDomain, 0)
			Ω(err).ShouldNot(HaveOccurred())
			defer func() { _ = te.DeleteProviderAndSecret(pr) }()
			providers = append(providers, pr)
			domains = append(domains, domain)

			e, err := te.CreateEntry(0, domain)
			Ω(err).ShouldNot(HaveOccurred())
			entries = append(entries, e)

			checkProviderEx(te, pr)

			checkEntryEx(te, e, pr)
		}
		Ω(providers).Should(HaveLen(2))
		Ω(entries).Should(HaveLen(2))

		for i, te := range envs {
			err = te.MockInMemoryHasEntry(entries[i])
			Ω(err).ShouldNot(HaveOccurred())
		}

		for i, te := range envs {
			providers[i], err = te.UpdateProviderSpec(providers[i], func(spec *v1alpha1.DNSProviderSpec) error {
				spec.Domains.Include = []string{"x." + baseDomain}
				spec.Domains.Exclude = []string{}
				return nil
			})
			Ω(err).ShouldNot(HaveOccurred())

			e := entries[i]
			err = te.AwaitEntryStale(e.GetName())
			Ω(err).ShouldNot(HaveOccurred())
		}

		for i, te := range envs {
			providers[i], err = te.UpdateProviderSpec(providers[i], func(spec *v1alpha1.DNSProviderSpec) error {
				spec.Domains.Include = []string{domains[i]}
				spec.Domains.Exclude = []string{}
				return nil
			})
			Ω(err).ShouldNot(HaveOccurred())

			e := entries[i]
			err = te.AwaitEntryReady(e.GetName())
			Ω(err).ShouldNot(HaveOccurred())
		}

		for i, te := range envs {
			err = te.DeleteEntryAndWait(entries[i])
			Ω(err).ShouldNot(HaveOccurred())

			err = te.DeleteProviderAndSecret(providers[i])
			Ω(err).ShouldNot(HaveOccurred())
		}
	})

	It("should deal with provider with two different private zones, but the same base domain", func() {
		secretName := testEnv.SecretName(0)
		_, err := testEnv.CreateSecret(0)
		Ω(err).ShouldNot(HaveOccurred())

		domain := "pr1.mock.xx"
		setSpec := func(provider *v1alpha1.DNSProvider) {
			spec := &provider.Spec
			spec.Zones = &v1alpha1.DNSSelection{Include: []string{"z1:private:" + domain, "z2:private:" + domain}}
			spec.Type = "local"

			var zonedata []local.MockZone
			for _, prefix := range []string{"z1:private:", "z2:private:"} {
				zonedata = append(zonedata, local.MockZone{
					ZonePrefix: prefix,
					DNSName:    domain,
				})
			}
			input := local.MockConfig{
				Name:  testEnv.Namespace,
				Zones: zonedata,
			}
			spec.ProviderConfig = testEnv.BuildProviderConfigEx(input)
			spec.SecretRef = &corev1.SecretReference{Name: secretName, Namespace: testEnv.Namespace}
		}

		pr, err := testEnv.CreateProviderEx(0, setSpec)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr)

		// duplicate base domain names are not allowed
		testEnv.AwaitProviderState(pr.GetName(), "Error")

		// should be ok to include only one zone
		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Zones.Include = []string{"z2:private:pr1.mock.xx"}
			return nil
		})
		Ω(err).ShouldNot(HaveOccurred())
		testEnv.AwaitProviderReady(pr.GetName())

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).ShouldNot(HaveOccurred())
		testEnv.AwaitEntryReady(e.GetName())
		err = testEnv.MockInMemoryHasEntryEx(testEnv.Namespace, "z2:private:", e)
		Ω(err).ShouldNot(HaveOccurred())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Zones.Include = []string{"z1:private:pr1.mock.xx"}
			return nil
		})
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(func(g Gomega) string {
			_, data, err := testEnv.GetProvider(pr.GetName())
			g.Expect(err).ShouldNot(HaveOccurred())
			return data.Status.Zones.Included[0]
		}).Should(Equal("z1:private:pr1.mock.xx"))

		testEnv.AwaitProviderReady(pr.GetName())

		Eventually(func(g Gomega) *string {
			obj, err := testEnv.GetEntry(e.GetName())
			g.Expect(err).ShouldNot(HaveOccurred())
			e := UnwrapEntry(obj)
			g.Expect(e.Status.State).Should(Equal("Ready"))
			return e.Status.Zone
		}).WithPolling(1 * time.Second).WithTimeout(15 * time.Second).Should(Equal(ptr.To("z1:private:pr1.mock.xx")))
		testEnv.AwaitEntryReady(e.GetName())
		Eventually(func() error {
			return testEnv.MockInMemoryHasEntryEx(testEnv.Namespace, "z1:private:", e)
		}).ShouldNot(HaveOccurred())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("should deal with two providers with different private zones", func() {
		pr1, domain1, _, err := testEnv.CreateSecretAndProvider("mock.xx", 1)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr1)
		pr2, domain2, _, err := testEnv.CreateSecretAndProvider("mock.xx", 2, AlternativeMockName)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr2)

		testEnv.AwaitProviderReady(pr1.GetName())
		testEnv.AwaitProviderReady(pr2.GetName())

		e, err := testEnv.CreateEntry(0, domain1)
		Ω(err).ShouldNot(HaveOccurred())
		testEnv.AwaitEntryReady(e.GetName())
		err = testEnv.MockInMemoryHasEntry(e)
		Ω(err).ShouldNot(HaveOccurred())

		_, err = testEnv.UpdateEntryDomain(e, fmt.Sprintf("e%d.%s", 0, domain2))
		Ω(err).ShouldNot(HaveOccurred())
		testEnv.AwaitEntryReady(e.GetName())

		var e2 resources.Object
		for i := 0; i < 25; i++ {
			e2, err = testEnv.GetEntry(e.GetName())
			Ω(err).ShouldNot(HaveOccurred())
			obj := UnwrapEntry(e2)
			if obj.Status.ObservedGeneration == obj.Generation {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		err = testEnv.MockInMemoryHasEntryEx(testEnv.Namespace+"-alt", testEnv.ZonePrefix, e2)
		Ω(err).ShouldNot(HaveOccurred())

		found := true
		for i := 0; i < 25; i++ {
			err = testEnv.MockInMemoryHasNotEntry(e)
			if err == nil {
				found = false
			}
			time.Sleep(500 * time.Millisecond)
		}
		Ω(found).Should(BeFalse())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.DeleteProviderAndSecret(pr1)
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.DeleteProviderAndSecret(pr2)
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("should complain about a provider with overlapping domains from two private zones", func() {
		secret, err := testEnv.CreateSecret(1)
		Ω(err).ShouldNot(HaveOccurred())

		setSpec := func(provider *v1alpha1.DNSProvider) {
			spec := &provider.Spec
			spec.Domains = &v1alpha1.DNSSelection{Include: []string{"a.mock.xx"}}
			spec.Type = "local"
			spec.ProviderConfig = testEnv.BuildProviderConfig("mock.xx", "a.mock.xx", PrivateZones)
			spec.SecretRef = &corev1.SecretReference{Name: secret.GetName(), Namespace: testEnv.Namespace}
		}

		pr1, err := testEnv.CreateProviderEx(1, setSpec)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr1)

		testEnv.AwaitProviderState(pr1.GetName(), "Error")

		_, pr1b, err := testEnv.GetProvider(pr1.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		Ω(pr1b.Status.Message).ShouldNot(BeNil())
		Ω(*pr1b.Status.Message).Should(ContainSubstring("overlapping zones"))

		err = testEnv.DeleteProviderAndSecret(pr1)
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("should complain about a provider with same domains from two private zones", func() {
		secret, err := testEnv.CreateSecret(1)
		Ω(err).ShouldNot(HaveOccurred())

		setSpec := func(provider *v1alpha1.DNSProvider) {
			spec := &provider.Spec
			spec.Domains = &v1alpha1.DNSSelection{Include: []string{"a.mock.xx"}}
			spec.Type = "local"
			spec.ProviderConfig = testEnv.BuildProviderConfig("mock.xx", "mock.xx", PrivateZones)
			spec.SecretRef = &corev1.SecretReference{Name: secret.GetName(), Namespace: testEnv.Namespace}
		}

		pr1, err := testEnv.CreateProviderEx(1, setSpec)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr1)

		testEnv.AwaitProviderState(pr1.GetName(), "Error")

		_, pr1b, err := testEnv.GetProvider(pr1.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		Ω(pr1b.Status.Message).ShouldNot(BeNil())
		Ω(*pr1b.Status.Message).Should(ContainSubstring("duplicate zones"))

		err = testEnv.DeleteProviderAndSecret(pr1)
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("should not complain about a provider with zones forming domain and forwareded subdomain", func() {
		secret, err := testEnv.CreateSecret(1)
		Ω(err).ShouldNot(HaveOccurred())

		setSpec := func(provider *v1alpha1.DNSProvider) {
			spec := &provider.Spec
			spec.Domains = &v1alpha1.DNSSelection{Include: []string{"mock.xx"}}
			spec.Type = "local"
			spec.ProviderConfig = testEnv.BuildProviderConfig("mock.xx", "sub.mock.xx", PrivateZones)
			spec.SecretRef = &corev1.SecretReference{Name: secret.GetName(), Namespace: testEnv.Namespace}
		}

		pr1, err := testEnv.CreateProviderEx(1, setSpec)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr1)

		testEnv.AwaitProviderReady(pr1.GetName())

		err = testEnv.DeleteProviderAndSecret(pr1)
		Ω(err).ShouldNot(HaveOccurred())
	})
})
