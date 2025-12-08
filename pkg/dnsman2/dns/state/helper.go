// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler"
)

// GetStandardDNSHandlerFactory returns a standard DNSHandlerFactory.
func GetStandardDNSHandlerFactory(cfg config.DNSProviderControllerConfig) provider.DNSHandlerFactory {
	s := GetState()
	factory := s.GetDNSHandlerFactory()
	if factory != nil {
		return factory
	}
	factory = handler.CreateStandardDNSHandlerFactory(cfg)
	return s.SetDNSHandlerFactoryOnce(factory)
}
