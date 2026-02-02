// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"golang.org/x/oauth2/google/externalaccount"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const externalAccountCredentialType = "external_account"

var projectIDRegexp = regexp.MustCompile(`^(?P<project>[a-z][a-z0-9-]{4,28}[a-z0-9])$`)

// WorkloadIdentityConfig contains configuration settings for GCP workload identity.
// copy from https://github.com/gardener/gardener-extension-provider-gcp/blob/6e2574698ed9e3b75734dcc5eaa7dd93c39f00fd/pkg/apis/gcp/v1alpha1/types_workloadidentity.go
type WorkloadIdentityConfig struct {
	metav1.TypeMeta `json:",inline"`

	// ProjectID is the ID of the GCP project.
	ProjectID string `json:"projectID,omitempty"`
	// CredentialsConfig contains information for workload authentication against GCP.
	CredentialsConfig *runtime.RawExtension `json:"credentialsConfig,omitempty"`
}

// DeepCopy creates a deep copy of the WorkloadIdentityConfig.
func (c *WorkloadIdentityConfig) DeepCopy() *WorkloadIdentityConfig {
	if c == nil {
		return nil
	}
	out := new(WorkloadIdentityConfig)
	out.TypeMeta = c.TypeMeta
	out.ProjectID = c.ProjectID
	if c.CredentialsConfig != nil {
		out.CredentialsConfig = c.CredentialsConfig.DeepCopy()
	}
	return out
}

type credConfig struct {
	Type string `json:"type"`

	Audience                       string           `json:"audience"`
	CredentialSource               credentialSource `json:"credential_source"`
	UniverseDomain                 string           `json:"universe_domain"`
	TokenURL                       string           `json:"token_url"`
	SubjectTokenType               string           `json:"subject_token_type"`
	ServiceAccountImpersonationURL string           `json:"service_account_impersonation_url"`
}

type credentialSource struct {
	File   string            `json:"file"`
	Format credentialsFormat `json:"format"`
}

type credentialsFormat struct {
	Type string `json:"type"`
}

type staticTokenSupplier struct {
	token string
}

var _ externalaccount.SubjectTokenSupplier = &staticTokenSupplier{}

func (s *staticTokenSupplier) SubjectToken(_ context.Context, _ externalaccount.SupplierOptions) (string, error) {
	return s.token, nil
}

// ExtractExternalAccountCredentials extracts external account credentials from the WorkloadIdentityConfig
// using the provided static token and scopes.
func (cfg *WorkloadIdentityConfig) ExtractExternalAccountCredentials(staticToken string, scopes ...string) (externalaccount.Config, error) {
	if cfg.CredentialsConfig == nil {
		return externalaccount.Config{}, fmt.Errorf("credentials config is nil")
	}
	credConfig := &credConfig{}
	if err := json.Unmarshal(cfg.CredentialsConfig.Raw, credConfig); err != nil {
		return externalaccount.Config{}, fmt.Errorf("could not unmarshal credentials config: %w", err)
	}
	if credConfig.Type != externalAccountCredentialType {
		return externalaccount.Config{}, fmt.Errorf("invalid credential type: expected %q, got %q", externalAccountCredentialType, credConfig.Type)
	}
	return externalaccount.Config{
		Audience:                       credConfig.Audience,
		SubjectTokenType:               credConfig.SubjectTokenType,
		TokenURL:                       credConfig.TokenURL,
		Scopes:                         scopes,
		SubjectTokenSupplier:           &staticTokenSupplier{token: staticToken},
		UniverseDomain:                 credConfig.UniverseDomain,
		ServiceAccountImpersonationURL: credConfig.ServiceAccountImpersonationURL,
	}, nil
}
