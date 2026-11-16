// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package gdc implements the dnsman2 provider handler for Google Distributed Cloud (GDC) air-gapped DNS.
package gdc

import (
	"encoding/json"
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/runtime"

	gdcclient "github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client"
	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/auth"
	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/constants"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// ProviderType is the type code for GDC DNS provider.
const ProviderType = "gdch-dns"

// RegisterTo registers the GDC DNS handler to the registry.
func RegisterTo(registry *provider.DNSHandlerRegistry, cfg config.DNSProviderControllerConfig) {
	var allowedTokenURIs, allowedOrgClusterURLs []string
	if cfg.GDCConfig != nil {
		allowedTokenURIs = cfg.GDCConfig.AllowedTokenURIs
		allowedOrgClusterURLs = cfg.GDCConfig.AllowedOrgClusterURLs
	}
	registry.Register(ProviderType, NewHandler, newAdapter(allowedTokenURIs, allowedOrgClusterURLs), &config.RateLimiterOptions{
		Enabled: true,
		QPS:     100,
		Burst:   20,
	}, nil)
}

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

var _ provider.DNSHandlerAdapter = &adapter{}

func newAdapter(allowedTokenURIs, allowedOrgClusterURLs []string) provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty(constants.ServiceAccountJSONField).Validators(func(value string) error {
		sa, err := validateServiceAccountJSON([]byte(value))
		if err != nil {
			return err
		}
		if err := provider.URLValidator("https")(sa.TokenURI); err != nil {
			return fmt.Errorf("'%s' field 'token_uri': %w", constants.ServiceAccountJSONField, err)
		}
		if err := provider.URLAllowListValidator(allowedTokenURIs)(sa.TokenURI); err != nil {
			return fmt.Errorf("'%s' field 'token_uri': %w", constants.ServiceAccountJSONField, err)
		}
		return nil
	}).HideValue())
	checks.Add(provider.RequiredProperty(constants.GDCHConfigJSONField).Validators(func(value string) error {
		var gdcConfig gdcclient.OrgClusterConfig
		if err := json.Unmarshal([]byte(value), &gdcConfig); err != nil {
			return fmt.Errorf("'%s' does not contain valid JSON: %w", constants.GDCHConfigJSONField, err)
		}
		if err := provider.URLValidator("https")(gdcConfig.OrgClusterURL); err != nil {
			return fmt.Errorf("'%s' field 'orgClusterURL': %w", constants.GDCHConfigJSONField, err)
		}
		if err := provider.URLAllowListValidator(allowedOrgClusterURLs)(gdcConfig.OrgClusterURL); err != nil {
			return fmt.Errorf("'%s' field 'orgClusterURL': %w", constants.GDCHConfigJSONField, err)
		}
		return nil
	}).HideValue())
	return &adapter{checks: checks}
}

func (a *adapter) ProviderType() string {
	return ProviderType
}

func (a *adapter) ValidateCredentialsAndProviderConfig(properties utils.Properties, config *runtime.RawExtension) error {
	if config != nil && len(config.Raw) > 0 {
		return fmt.Errorf("provider config not supported for %s provider", a.ProviderType())
	}
	return a.checks.ValidateProperties(a.ProviderType(), properties)
}

type lightCredentialsFile struct {
	Type string `json:"type"`

	// Service Account fields
	Name         string `json:"name"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"` // #nosec G117 - false positive
	Project      string `json:"project"`
}

var projectIDRegexp = regexp.MustCompile(`^(?P<project>[a-z][a-z0-9-]{4,30}[a-z0-9])$`)

func validateServiceAccountJSON(data []byte) (auth.ServiceAccount, error) {
	var credInfo auth.ServiceAccount
	var typeCheck struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(data, &typeCheck); err != nil {
		return credInfo, fmt.Errorf("'%s' data field does not contain a valid JSON: %s", constants.ServiceAccountJSONField, err)
	}
	if typeCheck.Type != "gdch_service_account" {
		return credInfo, fmt.Errorf("'%s' field 'type' is not 'gdch_service_account'", constants.ServiceAccountJSONField)
	}
	if err := json.Unmarshal(data, &credInfo); err != nil {
		return credInfo, fmt.Errorf("'%s' data field does not contain a valid JSON: %s", constants.ServiceAccountJSONField, err)
	}
	if !projectIDRegexp.MatchString(credInfo.Project) {
		return credInfo, fmt.Errorf("'%s' field 'project' is not a valid project", constants.ServiceAccountJSONField)
	}
	return credInfo, nil
}
