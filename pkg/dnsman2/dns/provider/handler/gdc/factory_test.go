// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gdc

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/utils/clock"

	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/constants"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

func TestRegisterAndFactory(t *testing.T) {
	registry := provider.NewDNSHandlerRegistry(clock.RealClock{})

	RegisterTo(registry)

	// Assert provider type support
	if !registry.Supports(ProviderType) {
		t.Fatalf("expected registry to support provider type %q", ProviderType)
	}

	// Assert adapter capabilities
	adapter, err := registry.GetDNSHandlerAdapter(ProviderType)
	if err != nil {
		t.Fatalf("unexpected error getting adapter: %v", err)
	}

	if diff := cmp.Diff(ProviderType, adapter.ProviderType()); diff != "" {
		t.Errorf("adapter ProviderType mismatch (-want +got):\n%s", diff)
	}

	if err := adapter.ValidateCredentialsAndProviderConfig(nil, nil); err != nil {
		t.Errorf("unexpected error validating empty credentials: %v", err)
	}

	// Test handler instantiation and Release
	config := &provider.DNSHandlerConfig{
		Properties: map[string]string{
			constants.GDCHConfigJSONField:     `{"orgClusterURL":"https://global-api.test"}`,
			constants.ServiceAccountJSONField: `{"project":"fake-project"}`,
		},
	}
	h, err := NewHandler(config)
	if err != nil {
		t.Fatalf("failed to create new handler: %v", err)
	}

	if diff := cmp.Diff(ProviderType, h.ProviderType()); diff != "" {
		t.Errorf("handler ProviderType mismatch (-want +got):\n%s", diff)
	}

	h.Release()
}
