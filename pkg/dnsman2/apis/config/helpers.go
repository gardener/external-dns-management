// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

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
