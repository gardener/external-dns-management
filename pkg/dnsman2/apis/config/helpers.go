// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"regexp"
)

// GetSourceClass returns the source class from the configuration, falling back to the general class if not set.
func GetSourceClass(cfg *DNSManagerConfiguration) string {
	if cfg.Controllers.Source.SourceClass != nil {
		return *cfg.Controllers.Source.SourceClass
	}
	return cfg.Class
}

// GetTargetClass returns the target class from the configuration, falling back to the general class if not set.
func GetTargetClass(cfg *DNSManagerConfiguration) string {
	if cfg.Controllers.Source.TargetClass != nil {
		return *cfg.Controllers.Source.TargetClass
	}
	return cfg.Class
}

// InternalGCPWorkloadIdentityConfig is the internal representation of GCPWorkloadIdentityConfig
// +k8s:deepcopy-gen=false
type InternalGCPWorkloadIdentityConfig struct {
	AllowedTokenURLs                             []string
	AllowedServiceAccountImpersonationURLRegExps []*regexp.Regexp
}

// NewInternalGCPWorkloadIdentityConfig creates an internal representation of GCPWorkloadIdentityConfig
func NewInternalGCPWorkloadIdentityConfig(cfg GCPWorkloadIdentityConfig) (*InternalGCPWorkloadIdentityConfig, error) {
	var regexps []*regexp.Regexp
	var errs []error
	for _, expr := range cfg.AllowedServiceAccountImpersonationURLRegExps {
		r, err := regexp.Compile(expr)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to compile allowed service account impersonation URL regexp '%s': %w", expr, err))
			continue
		}
		regexps = append(regexps, r)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("errors compiling GCP workload identity config: %v", errs)
	}
	return &InternalGCPWorkloadIdentityConfig{
		AllowedTokenURLs: cfg.AllowedTokenURLs,
		AllowedServiceAccountImpersonationURLRegExps: regexps,
	}, nil
}
