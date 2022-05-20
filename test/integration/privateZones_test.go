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
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/provider/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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
			Ω(err).Should(BeNil())
			defer te.DeleteProviderAndSecret(pr)
			providers = append(providers, pr)
			domains = append(domains, domain)

			e, err := te.CreateEntry(0, domain)
			Ω(err).Should(BeNil())
			entries = append(entries, e)

			checkProviderEx(te, pr)

			checkEntryEx(te, e, pr)
		}
		Ω(len(providers)).Should(Equal(2))
		Ω(len(entries)).Should(Equal(2))

		for i, te := range envs {
			err = te.MockInMemoryHasEntry(entries[i])
			Ω(err).Should(BeNil())
		}

		for i, te := range envs {
			providers[i], err = te.UpdateProviderSpec(providers[i], func(spec *v1alpha1.DNSProviderSpec) error {
				spec.Domains.Include = []string{"x." + baseDomain}
				spec.Domains.Exclude = []string{}
				return nil
			})
			Ω(err).Should(BeNil())

			e := entries[i]
			err = te.AwaitEntryError(e.GetName())
			Ω(err).Should(BeNil())
		}

		for i, te := range envs {
			providers[i], err = te.UpdateProviderSpec(providers[i], func(spec *v1alpha1.DNSProviderSpec) error {
				spec.Domains.Include = []string{domains[i]}
				spec.Domains.Exclude = []string{}
				return nil
			})
			Ω(err).Should(BeNil())

			e := entries[i]
			err = te.AwaitEntryReady(e.GetName())
			Ω(err).Should(BeNil())
		}

		for i, te := range envs {
			err = te.DeleteEntryAndWait(entries[i])
			Ω(err).Should(BeNil())

			err = te.DeleteProviderAndSecret(providers[i])
			Ω(err).Should(BeNil())
		}
	})

	It("should deal with provider with two different private zones, but the same base domain", func() {
		secretName := testEnv.SecretName(0)
		_, err := testEnv.CreateSecret(0)
		Ω(err).Should(BeNil())

		domain := "pr1.mock.xx"
		setSpec := func(provider *v1alpha1.DNSProvider) {
			spec := &provider.Spec
			spec.Zones = &v1alpha1.DNSSelection{Include: []string{"z1:" + domain, "z2" + domain}}
			spec.Type = "mock-inmemory"

			var zonedata []mock.MockZone
			for _, prefix := range []string{"z1:", "z2:"} {
				zonedata = append(zonedata, mock.MockZone{
					ZonePrefix: prefix,
					DNSName:    domain,
				})
			}
			input := mock.MockConfig{
				Name:  testEnv.Namespace,
				Zones: zonedata,
			}
			spec.ProviderConfig = testEnv.BuildProviderConfigEx(input)
			spec.SecretRef = &corev1.SecretReference{Name: secretName, Namespace: testEnv.Namespace}
		}

		pr, err := testEnv.CreateProviderEx(0, secretName, setSpec)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr)

		// duplicate base domain names are not allowed
		testEnv.AwaitProviderState(pr.GetName(), "Error")

		// should be ok to include only one zone
		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Zones.Include = []string{"z2:pr1.mock.xx"}
			return nil
		})
		Ω(err).Should(BeNil())
		testEnv.AwaitProviderReady(pr.GetName())

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).Should(BeNil())
		testEnv.AwaitEntryReady(e.GetName())
		err = testEnv.MockInMemoryHasEntryEx(testEnv.Namespace, "z2", e)
		Ω(err).Should(BeNil())

		pr, err = testEnv.UpdateProviderSpec(pr, func(spec *v1alpha1.DNSProviderSpec) error {
			spec.Zones.Include = []string{"z1:pr1.mock.xx"}
			return nil
		})
		Ω(err).Should(BeNil())
		found := false
		for i := 0; i < 20; i++ {
			time.Sleep(10 * time.Millisecond)
			var data *v1alpha1.DNSProvider
			pr, data, err = testEnv.GetProvider(pr.GetName())
			Ω(err).Should(BeNil())
			if data.Status.Zones.Included[0] == "z1:pr1.mock.xx" {
				found = true
				break
			}
		}
		Ω(found).Should(BeTrue())

		testEnv.AwaitProviderReady(pr.GetName())

		time.Sleep(100 * time.Millisecond)
		testEnv.AwaitEntryReady(e.GetName())
		err = testEnv.MockInMemoryHasEntryEx(testEnv.Namespace, "z1", e)
		Ω(err).Should(BeNil())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).Should(BeNil())
		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).Should(BeNil())
	})

	It("should deal with two providers with different private zones", func() {
		pr1, domain1, _, err := testEnv.CreateSecretAndProvider("mock.xx", 1)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr1)
		pr2, domain2, _, err := testEnv.CreateSecretAndProvider("mock.xx", 2, AlternativeMockName)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr2)

		testEnv.AwaitProviderReady(pr1.GetName())
		testEnv.AwaitProviderReady(pr2.GetName())

		e, err := testEnv.CreateEntry(0, domain1)
		Ω(err).Should(BeNil())
		testEnv.AwaitEntryReady(e.GetName())
		err = testEnv.MockInMemoryHasEntry(e)
		Ω(err).Should(BeNil())

		_, err = testEnv.UpdateEntryDomain(e, fmt.Sprintf("e%d.%s", 0, domain2))
		Ω(err).Should(BeNil())
		testEnv.AwaitEntryReady(e.GetName())

		var e2 resources.Object
		for i := 0; i < 25; i++ {
			e2, err = testEnv.GetEntry(e.GetName())
			Ω(err).Should(BeNil())
			obj := UnwrapEntry(e2)
			if obj.Status.ObservedGeneration == obj.Generation {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		err = testEnv.MockInMemoryHasEntryEx(testEnv.Namespace+"-alt", testEnv.ZonePrefix, e2)
		Ω(err).Should(BeNil())

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
		Ω(err).Should(BeNil())
		err = testEnv.DeleteProviderAndSecret(pr1)
		Ω(err).Should(BeNil())
		err = testEnv.DeleteProviderAndSecret(pr2)
		Ω(err).Should(BeNil())
	})

	It("should complain about a provider with overlapping domains from two private zones", func() {
		secret, err := testEnv.CreateSecret(1)
		Ω(err).Should(BeNil())

		setSpec := func(provider *v1alpha1.DNSProvider) {
			spec := &provider.Spec
			spec.Domains = &v1alpha1.DNSSelection{Include: []string{"a.mock.xx"}}
			spec.Type = "mock-inmemory"
			spec.ProviderConfig = testEnv.BuildProviderConfig("mock.xx", "a.mock.xx")
			spec.SecretRef = &corev1.SecretReference{Name: secret.GetName(), Namespace: testEnv.Namespace}
		}

		pr1, err := testEnv.CreateProviderEx(1, secret.GetName(), setSpec)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr1)

		testEnv.AwaitProviderState(pr1.GetName(), "Error")

		_, pr1b, err := testEnv.GetProvider(pr1.GetName())
		Ω(err).Should(BeNil())
		Ω(pr1b.Status.Message).ShouldNot(BeNil())
		Ω(*pr1b.Status.Message).Should(ContainSubstring("overlapping zones"))

		err = testEnv.DeleteProviderAndSecret(pr1)
		Ω(err).Should(BeNil())
	})

	It("should complain about a provider with same domains from two private zones", func() {
		secret, err := testEnv.CreateSecret(1)
		Ω(err).Should(BeNil())

		setSpec := func(provider *v1alpha1.DNSProvider) {
			spec := &provider.Spec
			spec.Domains = &v1alpha1.DNSSelection{Include: []string{"a.mock.xx"}}
			spec.Type = "mock-inmemory"
			spec.ProviderConfig = testEnv.BuildProviderConfig("mock.xx", "mock.xx")
			spec.SecretRef = &corev1.SecretReference{Name: secret.GetName(), Namespace: testEnv.Namespace}
		}

		pr1, err := testEnv.CreateProviderEx(1, secret.GetName(), setSpec)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr1)

		testEnv.AwaitProviderState(pr1.GetName(), "Error")

		_, pr1b, err := testEnv.GetProvider(pr1.GetName())
		Ω(err).Should(BeNil())
		Ω(pr1b.Status.Message).ShouldNot(BeNil())
		Ω(*pr1b.Status.Message).Should(ContainSubstring("duplicate zones"))

		err = testEnv.DeleteProviderAndSecret(pr1)
		Ω(err).Should(BeNil())
	})

	It("should not complain about a provider with zones forming domain and forwareded subdomain", func() {
		secret, err := testEnv.CreateSecret(1)
		Ω(err).Should(BeNil())

		setSpec := func(provider *v1alpha1.DNSProvider) {
			spec := &provider.Spec
			spec.Domains = &v1alpha1.DNSSelection{Include: []string{"mock.xx"}}
			spec.Type = "mock-inmemory"
			spec.ProviderConfig = testEnv.BuildProviderConfig("mock.xx", "sub.mock.xx", Domain2IsSubdomain)
			spec.SecretRef = &corev1.SecretReference{Name: secret.GetName(), Namespace: testEnv.Namespace}
		}

		pr1, err := testEnv.CreateProviderEx(1, secret.GetName(), setSpec)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr1)

		testEnv.AwaitProviderReady(pr1.GetName())

		err = testEnv.DeleteProviderAndSecret(pr1)
		Ω(err).Should(BeNil())
	})
})
