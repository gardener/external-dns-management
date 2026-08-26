// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package infoblox

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/controller/provider/infoblox/validation"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = validation.ProviderType

var Factory = provider.NewDNSHandlerFactory(NewHandler, validation.NewAdapter())

func init() {
	compound.MustRegister(Factory)
}
