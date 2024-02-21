// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package infoblox

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = "infoblox-dns"

var Factory = provider.NewDNSHandlerFactory(TYPE_CODE, NewHandler)

func init() {
	compound.MustRegister(Factory)
}
