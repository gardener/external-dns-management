// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"net"
	"time"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
)

var _ = Describe("EntryLivecycle", func() {
	It("has correct life cycle with provider", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())

		defer testEnv.DeleteProviderAndSecret(pr)

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).ShouldNot(HaveOccurred())

		checkProvider(pr)

		checkEntry(e, pr)

		// check ignore annotation
		orgTarget := UnwrapEntry(e).Spec.Targets[0]
		newTarget := "2" + orgTarget
		_, err = testEnv.UpdateEntry(e, func(entry *v1alpha1.DNSEntry) error {
			if entry.Annotations == nil {
				entry.Annotations = map[string]string{}
			}
			entry.Annotations["dns.gardener.cloud/ignore"] = "true"
			entry.Spec.Targets[0] = newTarget
			return nil
		})
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitEntryState(e.GetName(), "Ignored", "")
		Ω(err).ShouldNot(HaveOccurred())
		e, err = testEnv.GetEntry(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		Ω(UnwrapEntry(e).Status.Targets).To(Equal([]string{orgTarget}))
		err = testEnv.AnnotateObject(e, "dns.gardener.cloud/ignore", "")
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.AwaitEntryState(e.GetName(), "Ready", "")
		Ω(err).ShouldNot(HaveOccurred())
		e, err = testEnv.GetEntry(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())
		Ω(UnwrapEntry(e).Status.Targets).To(Equal([]string{newTarget}))

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryState(e.GetName(), "Error", "")
		Ω(err).ShouldNot(HaveOccurred())

		time.Sleep(10 * time.Second)

		err = testEnv.AwaitEntryState(e.GetName(), "Error")
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitFinalizers(e)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("has correct life cycle with provider for TXT record", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())

		defer testEnv.DeleteProviderAndSecret(pr)

		e, err := testEnv.CreateTXTEntry(0, domain+".")
		Ω(err).ShouldNot(HaveOccurred())

		checkProvider(pr)

		checkEntry(e, pr)

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryState(e.GetName(), "Error", "")
		Ω(err).ShouldNot(HaveOccurred())

		time.Sleep(10 * time.Second)

		err = testEnv.AwaitEntryState(e.GetName(), "Error")
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitFinalizers(e)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("handles an entry without targets as invalid and can delete it", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())

		defer testEnv.DeleteProviderAndSecret(pr)

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteEntryAndWait(e)

		checkProvider(pr)

		checkEntry(e, pr)

		e, err = testEnv.UpdateEntryTargets(e)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryInvalid(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.Await("entry still in mock provider", func() (bool, error) {
			err := testEnv.MockInMemoryHasNotEntry(e)
			return err == nil, err
		})
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("handles entry correctly from ready -> invalid -> ready", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())

		defer testEnv.DeleteProviderAndSecret(pr)

		e, err := testEnv.CreateEntry(0, domain)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteEntryAndWait(e)

		checkProvider(pr)

		checkEntry(e, pr)

		e, err = testEnv.UpdateEntryTargets(e)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryInvalid(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		e, err = testEnv.UpdateEntryTargets(e, "1.1.1.1")
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("is handled only by matching provider", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())

		defer testEnv.DeleteProviderAndSecret(pr)

		e, err := testEnv.CreateEntry(0, domain)
		dnsName := UnwrapEntry(e).Spec.DNSName
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteEntryAndWait(e)

		checkProvider(pr)

		checkEntry(e, pr)

		e, err = testEnv.UpdateEntryDomain(e, "foo.mock")
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryState(e.GetName(), "Error")
		Ω(err).ShouldNot(HaveOccurred())

		e, err = testEnv.UpdateEntryDomain(e, dnsName)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.AwaitEntryReady(e.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteEntryAndWait(e)
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("handles entry with multiple cname targets/resolveTargetsToAddresses correctly", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())

		defer testEnv.DeleteProviderAndSecret(pr)

		index := 0
		ttl := int64(300)
		setSpec := func(e *v1alpha1.DNSEntry) {
			e.Spec.TTL = &ttl
			e.Spec.DNSName = fmt.Sprintf("e%d.%s", index, domain)
			e.Spec.Targets = []string{
				"wikipedia.org",
				"www.wikipedia.org",
				"wikipedia.com",
				"www.wikipedia.com",
			}
		}
		e0, err := testEnv.CreateEntryGeneric(index, setSpec)
		Ω(err).ShouldNot(HaveOccurred())

		index = 1
		setSpec = func(e *v1alpha1.DNSEntry) {
			e.Spec.TTL = &ttl
			e.Spec.DNSName = fmt.Sprintf("e%d.%s", index, domain)
			e.Spec.Targets = []string{
				"www.wikipedia.org",
			}
			e.Spec.ResolveTargetsToAddresses = ptr.To(true)
		}
		e1, err := testEnv.CreateEntryGeneric(index, setSpec)
		Ω(err).ShouldNot(HaveOccurred())

		checkProvider(pr)

		By("check deduplication", func() {
			entry := checkEntry(e0, pr)
			targets := utils.NewStringSet(entry.Status.Targets...)
			Ω(targets).To(HaveLen(len(entry.Status.Targets))) // no duplicates
		})

		By("check single target with resolveTargetsToAddresses", func() {
			entry := checkEntry(e1, pr)
			Ω(entry.Status.Targets).NotTo(BeEmpty())
			for _, target := range entry.Status.Targets {
				Ω(net.ParseIP(target)).NotTo(BeNil())
			}
			Ω(entry.Status.CNameLookupInterval).NotTo(BeNil())
		})

		err = testEnv.DeleteEntryAndWait(e0)
		Ω(err).ShouldNot(HaveOccurred())
		err = testEnv.DeleteEntryAndWait(e1)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).ShouldNot(HaveOccurred())
	})

	It("handles entry with invalid domain name correctly", func() {
		pr, domain, _, err := testEnv.CreateSecretAndProvider("inmemory.mock", 0)
		Ω(err).ShouldNot(HaveOccurred())

		defer testEnv.DeleteProviderAndSecret(pr)

		setSpec := func(e *v1alpha1.DNSEntry) {
			e.Spec.DNSName = fmt.Sprintf("invalid-*.%s", domain)
			e.Spec.Targets = []string{"1.2.3.4"}
		}
		e0, err := testEnv.CreateEntryGeneric(0, setSpec)
		Ω(err).ShouldNot(HaveOccurred())

		checkProvider(pr)

		Ω(testEnv.AwaitEntryInvalid(e0.GetName())).ShouldNot(HaveOccurred())

		err = testEnv.DeleteEntryAndWait(e0)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).ShouldNot(HaveOccurred())
	})
})
