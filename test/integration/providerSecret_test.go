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

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/provider/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

func createAndDelete() {
	secretName := testEnv.SecretName(0)
	pr, _, _, err := testEnv.CreateProvider("inmemory.mock", 0, secretName)
	Ω(err).Should(BeNil())
	defer testEnv.DeleteProviderAndSecret(pr)

	checkHasFinalizer(pr)

	err = testEnv.AwaitProviderState(pr.GetName(), "Error")
	Ω(err).Should(BeNil())

	// create secret after provider
	secret, err := testEnv.CreateSecret(0)
	Ω(err).Should(BeNil())

	// provider should be ready now
	checkProvider(pr)

	checkHasFinalizer(secret)
}

var _ = Describe("ProviderSecret", func() {
	It("works if secret is created after provider", func() {
		Context("first round", createAndDelete)

		secretName := testEnv.SecretName(0)
		err := testEnv.AwaitSecretDeletion(secretName)
		Ω(err).Should(BeNil())

		Context("second round", createAndDelete)
	})

	It("takes into account includes and excludes of domain names and zone ids", func() {
		secretName := testEnv.SecretName(0)
		_, err := testEnv.CreateSecret(0)
		Ω(err).Should(BeNil())

		var zonedata []mock.MockZone
		for _, c := range []string{"a", "b", "c", "d", "e"} {
			zonedata = append(zonedata, mock.MockZone{
				ZonePrefix: testEnv.ZonePrefix,
				DNSName:    fmt.Sprintf("pr1%s.mock.xx", c),
			})
		}

		prefix := testEnv.ZonePrefix
		setSpec := func(provider *v1alpha1.DNSProvider) {
			spec := &provider.Spec
			spec.Domains = &v1alpha1.DNSSelection{Include: []string{"pr1a.mock.xx", "pr1b.mock.xx"}, Exclude: []string{"pr1d.mock.xx"}}
			spec.Zones = &v1alpha1.DNSSelection{Include: []string{prefix + "pr1a.mock.xx", prefix + "pr1c.mock.xx"}, Exclude: []string{prefix + "pr1e.mock.xx"}}
			spec.Type = "mock-inmemory"
			input := mock.MockConfig{
				Name:  testEnv.Namespace,
				Zones: zonedata,
			}
			spec.ProviderConfig = testEnv.BuildProviderConfigEx(input)
			spec.SecretRef = &corev1.SecretReference{Name: secretName, Namespace: testEnv.Namespace}
		}

		pr, err := testEnv.CreateProviderEx(1, secretName, setSpec)
		Ω(err).Should(BeNil())
		defer testEnv.DeleteProviderAndSecret(pr)

		checkProvider(pr)

		_, data, err := testEnv.GetProvider(pr.GetName())
		Ω(err).Should(BeNil())

		Ω(data.Status.Zones.Included).Should(ConsistOf(prefix + "pr1a.mock.xx"))
		Ω(data.Status.Zones.Excluded).Should(ConsistOf(prefix+"pr1b.mock.xx", prefix+"pr1c.mock.xx", prefix+"pr1d.mock.xx", prefix+"pr1e.mock.xx"))
		Ω(data.Status.Domains.Included).Should(ConsistOf("pr1a.mock.xx"))
	})
})
