// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/controller/provider/compound"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = "mock-inmemory"

var rateLimiterDefaults = provider.RateLimiterOptions{
	Enabled: true,
	QPS:     100,
	Burst:   20,
}

var Factory = provider.NewDNSHandlerFactory(NewHandler, &adapter{}).
	SetGenericFactoryOptionDefaults(provider.GenericFactoryOptionDefaults.SetRateLimiterOptions(rateLimiterDefaults))

func init() {
	compound.MustRegister(Factory)
}

type adapter struct {
}

func (a *adapter) ProviderType() string {
	return TYPE_CODE
}

func (a *adapter) ValidateCredentialsAndProviderConfig(props utils.Properties, _ *runtime.RawExtension) error {
	// no validation as it is only used for testing

	// for validation testing the property "bad_key" is used to force an error
	if value, ok := props["bad_key"]; ok {
		return fmt.Errorf("'bad_key' is not allowed in mock provider properties: %s", value)
	}
	return nil
}
