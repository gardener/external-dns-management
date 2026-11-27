// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var _ = Describe("Adapter", func() {
	props := func(authURL string) utils.Properties {
		return utils.Properties{"OS_AUTH_URL": authURL, "OS_USERNAME": "u", "OS_PASSWORD": "p"}
	}

	It("accepts any auth URL when no allow list is configured", func() {
		a := newAdapter(nil)
		Expect(a.ValidateCredentialsAndProviderConfig(props("http://169.254.169.254/v3"), nil)).To(Succeed())
	})

	It("accepts an auth URL on an allowed origin", func() {
		a := newAdapter([]string{"https://keystone.example.com"})
		Expect(a.ValidateCredentialsAndProviderConfig(props("https://keystone.example.com/v3"), nil)).To(Succeed())
	})

	It("rejects an auth URL outside the allow list", func() {
		a := newAdapter([]string{"https://keystone.example.com"})
		Expect(a.ValidateCredentialsAndProviderConfig(props("http://169.254.169.254/v3"), nil)).ToNot(Succeed())
	})
})
