// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gdc

import (
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// ProviderType is the type code for GDC DNS provider.
const ProviderType = "gdch-dns"

// RegisterTo registers the GDC DNS handler to the registry.
func RegisterTo(registry *provider.DNSHandlerRegistry) {
	registry.Register(ProviderType, NewHandler, &adapter{}, &config.RateLimiterOptions{
		Enabled: true,
		QPS:     100,
		Burst:   20,
	}, nil)
}

type adapter struct{}

var _ provider.DNSHandlerAdapter = &adapter{}

func (a *adapter) ProviderType() string {
	return ProviderType
}

func (a *adapter) ValidateCredentialsAndProviderConfig(_ utils.Properties, _ *runtime.RawExtension) error {
	return nil
}
