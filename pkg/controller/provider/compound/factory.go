// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package compound

import "github.com/gardener/external-dns-management/pkg/dns/provider"

const NAME = "compound"

var Factory = provider.NewDNSHandlerCompoundFactory(NAME)

func Register(fac provider.DNSHandlerFactory, finalizer ...string) error {
	return Factory.Add(fac, finalizer...)
}

func MustRegister(fac provider.DNSHandlerFactory, finalizer ...string) {
	if err := Factory.Add(fac, finalizer...); err != nil {
		panic(err)
	}
}
