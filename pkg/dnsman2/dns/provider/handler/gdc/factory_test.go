// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package gdc

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/clock"

	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/constants"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

func TestRegisterAndFactory(t *testing.T) {
	registry := provider.NewDNSHandlerRegistry(clock.RealClock{})

	RegisterTo(registry, config.DNSProviderControllerConfig{})

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

	// Should fail with nil / empty properties
	if err := adapter.ValidateCredentialsAndProviderConfig(nil, nil); err == nil {
		t.Errorf("expected error validating empty credentials, got nil")
	}

	validServiceAccountJSON := `{"type":"gdch_service_account","project":"fake-project","name":"dns-sa","private_key_id":"key-1","token_uri":"https://sts.test/token"}`
	validGDCHConfigJSON := `{"orgClusterURL":"https://global-api.test"}`

	validProps := utils.Properties{
		constants.ServiceAccountJSONField: validServiceAccountJSON,
		constants.GDCHConfigJSONField:     validGDCHConfigJSON,
	}

	if err := adapter.ValidateCredentialsAndProviderConfig(validProps, nil); err != nil {
		t.Errorf("expected valid properties to succeed, got: %v", err)
	}

	// Should reject provider config if provided
	if err := adapter.ValidateCredentialsAndProviderConfig(validProps, &runtime.RawExtension{Raw: []byte(`{"foo":"bar"}`)}); err == nil {
		t.Errorf("expected error when provider config is provided, got nil")
	}

	// Should reject http scheme in token_uri
	httpTokenURIProps := utils.Properties{
		constants.ServiceAccountJSONField: `{"type":"gdch_service_account","project":"fake-project","name":"dns-sa","private_key_id":"key-1","token_uri":"http://sts.test/token"}`,
		constants.GDCHConfigJSONField:     validGDCHConfigJSON,
	}
	if err := adapter.ValidateCredentialsAndProviderConfig(httpTokenURIProps, nil); err == nil {
		t.Errorf("expected error for http token_uri, got nil")
	}

	// Should reject http scheme in orgClusterURL
	httpOrgClusterURLProps := utils.Properties{
		constants.ServiceAccountJSONField: validServiceAccountJSON,
		constants.GDCHConfigJSONField:     `{"orgClusterURL":"http://global-api.test"}`,
	}
	if err := adapter.ValidateCredentialsAndProviderConfig(httpOrgClusterURLProps, nil); err == nil {
		t.Errorf("expected error for http orgClusterURL, got nil")
	}

	// Test handler instantiation and Release
	handlerCfg := &provider.DNSHandlerConfig{
		Properties: map[string]string{
			constants.GDCHConfigJSONField:     validGDCHConfigJSON,
			constants.ServiceAccountJSONField: validServiceAccountJSON,
		},
	}
	h, err := NewHandler(handlerCfg)
	if err != nil {
		t.Fatalf("failed to create new handler: %v", err)
	}

	if diff := cmp.Diff(ProviderType, h.ProviderType()); diff != "" {
		t.Errorf("handler ProviderType mismatch (-want +got):\n%s", diff)
	}

	h.Release()
}

func TestAllowListValidation(t *testing.T) {
	registry := provider.NewDNSHandlerRegistry(clock.RealClock{})
	RegisterTo(registry, config.DNSProviderControllerConfig{
		GDCConfig: &config.GDCConfig{
			AllowedTokenURIs:      []string{"https://sts.allowed.test"},
			AllowedOrgClusterURLs: []string{"https://global-api.allowed.test"},
		},
	})

	adapter, err := registry.GetDNSHandlerAdapter(ProviderType)
	if err != nil {
		t.Fatalf("unexpected error getting adapter: %v", err)
	}

	allowedProps := utils.Properties{
		constants.ServiceAccountJSONField: `{"type":"gdch_service_account","project":"fake-project","name":"dns-sa","private_key_id":"key-1","token_uri":"https://sts.allowed.test/token"}`,
		constants.GDCHConfigJSONField:     `{"orgClusterURL":"https://global-api.allowed.test"}`,
	}
	if err := adapter.ValidateCredentialsAndProviderConfig(allowedProps, nil); err != nil {
		t.Errorf("expected allowed URLs to pass, got: %v", err)
	}

	blockedTokenURIProps := utils.Properties{
		constants.ServiceAccountJSONField: `{"type":"gdch_service_account","project":"fake-project","name":"dns-sa","private_key_id":"key-1","token_uri":"https://sts.evil.test/token"}`,
		constants.GDCHConfigJSONField:     `{"orgClusterURL":"https://global-api.allowed.test"}`,
	}
	if err := adapter.ValidateCredentialsAndProviderConfig(blockedTokenURIProps, nil); err == nil {
		t.Errorf("expected error for token_uri not in allowlist, got nil")
	}

	blockedOrgClusterURLProps := utils.Properties{
		constants.ServiceAccountJSONField: `{"type":"gdch_service_account","project":"fake-project","name":"dns-sa","private_key_id":"key-1","token_uri":"https://sts.allowed.test/token"}`,
		constants.GDCHConfigJSONField:     `{"orgClusterURL":"https://global-api.evil.test"}`,
	}
	if err := adapter.ValidateCredentialsAndProviderConfig(blockedOrgClusterURLProps, nil); err == nil {
		t.Errorf("expected error for orgClusterURL not in allowlist, got nil")
	}
}
