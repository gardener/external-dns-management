// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"regexp"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

// ProviderType is the type code for the GDC-ag DNS provider.
const ProviderType = "gdch-dns"

// LightCredentialsFile represents a minimal set of fields required for GDC-ag DNS service account credentials.
type LightCredentialsFile struct {
	Type string `json:"type"`

	// Service Account fields
	Name         string `json:"name"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	Project      string `json:"project"`
}

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

// NewAdapter creates a new DNSHandlerAdapter for the Google Cloud DNS provider.
func NewAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty("serviceaccount.json").Validators(func(value string) error {
		_, err := ValidateServiceAccountJSON([]byte(value))
		return err
	}).HideValue())
	checks.Add(provider.RequiredProperty("gdch-config").HideValue())
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

var projectIDRegexp = regexp.MustCompile(`^(?P<project>[a-z][a-z0-9-]{4,30}[a-z0-9])$`)

func ValidateServiceAccountJSON(data []byte) (LightCredentialsFile, error) {
	var credInfo LightCredentialsFile

	if err := json.Unmarshal(data, &credInfo); err != nil {
		return credInfo, fmt.Errorf("'serviceaccount.json' data field does not contain a valid JSON: %s", err)
	}
	if !projectIDRegexp.MatchString(credInfo.Project) {
		return credInfo, fmt.Errorf("'serviceaccount.json' field 'project' is not a valid project")
	}
	if credInfo.Type != "service_account" {
		return credInfo, fmt.Errorf("'serviceaccount.json' field 'type' is not 'service_account'")
	}
	return credInfo, nil
}
