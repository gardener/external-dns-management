// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/provider/mock"
)

var _ = Describe("ProviderSecret", func() {
	createAndDelete := func() {
		secretName := testEnv.SecretName(0)
		pr, _, _, err := testEnv.CreateProvider("inmemory.mock", 0, secretName)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr)

		checkHasFinalizer(pr)

		err = testEnv.AwaitProviderState(pr.GetName(), "Error")
		Ω(err).ShouldNot(HaveOccurred())

		// create secret after provider
		secret, err := testEnv.CreateSecret(0)
		Ω(err).ShouldNot(HaveOccurred())

		// provider should be ready now
		checkProvider(pr)

		checkHasFinalizer(secret)
	}

	It("works if secret is created after provider", func() {
		By("first round", createAndDelete)

		secretName := testEnv.SecretName(0)
		err := testEnv.AwaitSecretDeletion(secretName)
		Ω(err).ShouldNot(HaveOccurred())

		By("second round", createAndDelete)
	})

	It("takes into account includes and excludes of domain names and zone ids", func() {
		secretName := testEnv.SecretName(0)
		_, err := testEnv.CreateSecret(0)
		Ω(err).ShouldNot(HaveOccurred())

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

		pr, err := testEnv.CreateProviderEx(1, setSpec)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr)

		checkProvider(pr)

		_, data, err := testEnv.GetProvider(pr.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		Ω(data.Status.Zones.Included).Should(ConsistOf(prefix + "pr1a.mock.xx"))
		Ω(data.Status.Zones.Excluded).Should(ConsistOf(prefix+"pr1b.mock.xx", prefix+"pr1c.mock.xx", prefix+"pr1d.mock.xx", prefix+"pr1e.mock.xx"))
		Ω(data.Status.Domains.Included).Should(ConsistOf("pr1a.mock.xx"))
	})
})
