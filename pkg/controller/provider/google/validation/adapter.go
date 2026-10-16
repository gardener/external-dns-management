// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/gardener/controller-manager-library/pkg/utils"
	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

// ProviderType is the type code for the Google Cloud DNS provider.
const ProviderType = "google-clouddns"

// LightCredentialsFile represents a minimal set of fields required for Google Cloud DNS service account credentials.
type LightCredentialsFile struct {
	Type string `json:"type"`

	// Service Account fields
	ClientEmail  string `json:"client_email"`
	PrivateKeyID string `json:"private_key_id"`
	ProjectID    string `json:"project_id"`
	TokenURI     string `json:"token_uri"`
}

// allowedTokenURIs are the Google OAuth token endpoints a service account may
// point its token_uri at.
var allowedTokenURIs = []string{
	"https://oauth2.googleapis.com/token",
	"https://accounts.google.com/o/oauth2/token",
}

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

// NewAdapter creates a new DNSHandlerAdapter for the Google Cloud DNS provider.
func NewAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.SetDisjunctPropertySets([]string{"serviceaccount.json"},
		[]string{securityv1alpha1constants.DataKeyToken, securityv1alpha1constants.DataKeyConfig, securityv1alpha1constants.LabelWorkloadIdentityProvider})
	checks.Add(provider.OptionalProperty("serviceaccount.json").Validators(func(value string) error {
		_, err := ValidateServiceAccountJSON([]byte(value))
		return err
	}).HideValue())
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.DataKeyToken).
		Validators(provider.MaxLengthValidator(4096)))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.DataKeyConfig).
		Validators(provider.MaxLengthValidator(4096)))
	checks.Add(provider.OptionalProperty(securityv1alpha1constants.LabelWorkloadIdentityProvider).
		Validators(provider.ExpectedValueValidator("gcp")))
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

var projectIDRegexp = regexp.MustCompile(`^(?P<project>[a-z][a-z0-9-]{4,28}[a-z0-9])$`)

func ValidateServiceAccountJSON(data []byte) (LightCredentialsFile, error) {
	var credInfo LightCredentialsFile

	if err := json.Unmarshal(data, &credInfo); err != nil {
		return credInfo, fmt.Errorf("'serviceaccount.json' data field does not contain a valid JSON: %s", err)
	}
	if !projectIDRegexp.MatchString(credInfo.ProjectID) {
		return credInfo, fmt.Errorf("'serviceaccount.json' field 'project_id' is not a valid project")
	}
	if credInfo.Type != "service_account" {
		return credInfo, fmt.Errorf("'serviceaccount.json' field 'type' is not 'service_account'")
	}
	// token_uri is optional; when empty the oauth2 library defaults to the Google
	// endpoint. When set it must be a known Google token endpoint.
	if credInfo.TokenURI != "" && !slices.Contains(allowedTokenURIs, credInfo.TokenURI) {
		return credInfo, fmt.Errorf("'serviceaccount.json' field 'token_uri' is not an allowed value, must be one of %q", allowedTokenURIs)
	}
	return credInfo, nil
}
