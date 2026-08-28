// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package gdc

import (
	"encoding/json"
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/constants"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// ProviderType is the type code for GDC DNS provider.
const ProviderType = "gdch-dns"

// RegisterTo registers the GDC DNS handler to the registry.
func RegisterTo(registry *provider.DNSHandlerRegistry) {
	registry.Register(ProviderType, NewHandler, newAdapter(), &config.RateLimiterOptions{
		Enabled: true,
		QPS:     100,
		Burst:   20,
	}, nil)
}

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

var _ provider.DNSHandlerAdapter = &adapter{}

func newAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty(constants.ServiceAccountJSONField).Validators(func(value string) error {
		_, err := validateServiceAccountJSON([]byte(value))
		return err
	}).HideValue())
	checks.Add(provider.RequiredProperty(constants.GDCHConfigJSONField).HideValue())
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

func validateServiceAccountJSON(data []byte) (lightCredentialsFile, error) {
	var credInfo lightCredentialsFile

	if err := json.Unmarshal(data, &credInfo); err != nil {
		return credInfo, fmt.Errorf("'%s' data field does not contain a valid JSON: %s", constants.ServiceAccountJSONField, err)
	}
	if !projectIDRegexp.MatchString(credInfo.Project) {
		return credInfo, fmt.Errorf("'%s' field 'project' is not a valid project", constants.ServiceAccountJSONField)
	}
	if credInfo.Type != "gdch_service_account" {
		return credInfo, fmt.Errorf("'%s' field 'type' is not 'gdch_service_account'", constants.ServiceAccountJSONField)
	}
	return credInfo, nil
}
