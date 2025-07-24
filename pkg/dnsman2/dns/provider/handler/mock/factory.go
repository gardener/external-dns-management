// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// ProviderType is the identifier for the mock in-memory DNS provider.
const ProviderType = "mock-inmemory"

// RegisterTo registers the mock DNS handler to the given registry.
func RegisterTo(registry *provider.DNSHandlerRegistry) {
	registry.Register(ProviderType, NewHandler, &adapter{}, nil, nil)
}

type adapter struct {
}

func (a *adapter) ProviderType() string {
	return ProviderType
}

func (a *adapter) ValidateCredentialsAndProviderConfig(props utils.Properties, _ *runtime.RawExtension) error {
	// no validation as it is only used for testing

	// for validation testing the property "bad_key" is used to force an error
	if value, ok := props["bad_key"]; ok {
		return fmt.Errorf("'bad_key' is not allowed in mock provider properties: %s", value)
	}
	return nil
}
