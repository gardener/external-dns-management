// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"net"
	"time"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DNSActivation", func() {
	lookupName := "lock.mock.xx"
	lookupValues := []string{}

	// mock DNS TXT record lookup
	BeforeEach(func() {
		dnsutils.DNSActivationLookupTXTFunc = func(dnsname string) ([]string, error) {
			if dnsname == lookupName {
				return lookupValues, nil
			}
			return []string{}, nil
		}
	})
	AfterEach(func() {
		dnsutils.DNSActivationLookupTXTFunc = net.LookupTXT
	})

	It("should active/deactivate DNSOwner and related DNSEntries", func() {
		baseDomain := "xx.mock"

		pr, domain, _, err := testEnv.CreateSecretAndProvider(baseDomain, 0)
		Ω(err).ShouldNot(HaveOccurred())
		defer testEnv.DeleteProviderAndSecret(pr)

		ownerID := "id-owner1"
		setSpec := func(e *v1alpha1.DNSEntry) {
			var ttl int64 = 120
			e.Spec.TTL = &ttl
			e.Spec.DNSName = fmt.Sprintf("e1.%s", domain)
			e.Spec.Targets = []string{"1.1.1.1"}
			e.Spec.OwnerId = &ownerID
		}
		entry, err := testEnv.CreateEntryGeneric(0, setSpec)
		Ω(err).ShouldNot(HaveOccurred())

		checkProviderEx(testEnv, pr)

		err = testEnv.AwaitEntryStale(entry.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		clusterID := "cluster-id-1234"
		ownerSetSpec := func(o *v1alpha1.DNSOwner) {
			o.Spec.OwnerId = ownerID
			o.Spec.DNSActivation = &v1alpha1.DNSActivation{
				DNSName: lookupName,
				Value:   &clusterID,
			}
		}
		owner1, err := testEnv.CreateOwnerGeneric("owner1", ownerSetSpec)
		Ω(err).ShouldNot(HaveOccurred())

		active := false
		lookupValues = []string{clusterID}
		for i := 0; i < 30; i++ {
			time.Sleep(500 * time.Millisecond)
			obj, err := testEnv.GetOwner(owner1.GetName())
			Ω(err).ShouldNot(HaveOccurred())
			owner := UnwrapOwner(obj)
			if owner.Status.Active != nil && *owner.Status.Active {
				active = true
				break
			}
		}
		Ω(active).Should(BeTrue())

		err = testEnv.AwaitEntryReady(entry.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		lookupValues = []string{"foo"}
		for i := 0; i < 30; i++ {
			time.Sleep(500 * time.Millisecond)
			obj, err := testEnv.GetOwner(owner1.GetName())
			Ω(err).ShouldNot(HaveOccurred())
			owner := UnwrapOwner(obj)
			if owner.Status.Active != nil && !*owner.Status.Active {
				active = false
				break
			}
		}
		Ω(active).Should(BeFalse())

		err = testEnv.AwaitEntryStale(entry.GetName())
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteOwner(owner1)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteEntryAndWait(entry)
		Ω(err).ShouldNot(HaveOccurred())

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).ShouldNot(HaveOccurred())
	})
})
