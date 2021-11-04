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
	"net"
	"time"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DNSActivation", func() {
	var lookupName = "lock.mock.xx"
	var lookupValues = []string{}

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
		Ω(err).Should(BeNil())
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
		Ω(err).Should(BeNil())

		checkProviderEx(testEnv, pr)

		err = testEnv.AwaitEntryStale(entry.GetName())
		Ω(err).Should(BeNil())

		clusterID := "cluster-id-1234"
		ownerSetSpec := func(o *v1alpha1.DNSOwner) {
			o.Spec.OwnerId = ownerID
			o.Spec.DNSActivation = &v1alpha1.DNSActivation{
				DNSName: lookupName,
				Value:   &clusterID,
			}
		}
		owner1, err := testEnv.CreateOwnerGeneric("owner1", ownerSetSpec)
		Ω(err).Should(BeNil())

		active := false
		lookupValues = []string{clusterID}
		for i := 0; i < 30; i++ {
			time.Sleep(500 * time.Millisecond)
			obj, err := testEnv.GetOwner(owner1.GetName())
			Ω(err).Should(BeNil())
			owner := UnwrapOwner(obj)
			if owner.Status.Active != nil && *owner.Status.Active {
				active = true
				break
			}
		}
		Ω(active).Should(BeTrue())

		err = testEnv.AwaitEntryReady(entry.GetName())
		Ω(err).Should(BeNil())

		lookupValues = []string{"foo"}
		for i := 0; i < 30; i++ {
			time.Sleep(500 * time.Millisecond)
			obj, err := testEnv.GetOwner(owner1.GetName())
			Ω(err).Should(BeNil())
			owner := UnwrapOwner(obj)
			if owner.Status.Active != nil && !*owner.Status.Active {
				active = false
				break
			}
		}
		Ω(active).Should(BeFalse())

		err = testEnv.AwaitEntryStale(entry.GetName())
		Ω(err).Should(BeNil())

		err = testEnv.DeleteOwner(owner1)
		Ω(err).Should(BeNil())

		err = testEnv.DeleteEntryAndWait(entry)
		Ω(err).Should(BeNil())

		err = testEnv.DeleteProviderAndSecret(pr)
		Ω(err).Should(BeNil())
	})
})
