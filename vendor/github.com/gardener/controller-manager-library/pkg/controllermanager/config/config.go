/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved.
 * This file is licensed under the Apache Software License, v. 2 except as noted
 * otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package config

import (
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/configmain"
)

var GracePeriod time.Duration

const OPTION_SOURCE = "controllermanager"

type Config struct {
	Name                        string
	Maintainer                  string
	DisableNamespaceRestriction bool
	NamespaceRestriction        bool

	config.OptionSet
}

var _ config.OptionSource = (*Config)(nil)

func NewConfig() *Config {
	cfg := &Config{
		OptionSet: config.NewDefaultOptionSet(OPTION_SOURCE, ""),
	}
	cfg.AddDurationOption(&GracePeriod, "grace-period", "", 0, "inactivity grace period for detecting end of cleanup for shutdown")
	cfg.AddStringOption(&cfg.Name, "name", "", "", "name used for controller manager")
	cfg.AddBoolOption(&cfg.NamespaceRestriction, "namespace-local-access-only", "n", false, "enable access restriction for namespace local access only (deprecated)")
	cfg.AddBoolOption(&cfg.DisableNamespaceRestriction, "disable-namespace-restriction", "", false, "disable access restriction for namespace local access only")
	cfg.AddStringOption(&cfg.Maintainer, "maintainer", "", "", "maintainer key for crds (defaulted by manager name)")
	return cfg
}

func (this *Config) Evaluate() error {
	if this.NamespaceRestriction && this.DisableNamespaceRestriction {
		return fmt.Errorf("contradicting options given for namespace restriction")
	}
	if !this.DisableNamespaceRestriction {
		this.NamespaceRestriction = true
	}
	this.DisableNamespaceRestriction = false
	if this.Maintainer == "" {
		this.Maintainer = this.Name
	}
	return this.OptionSet.Evaluate()
}

func GetConfig(cfg *configmain.Config) *Config {
	return cfg.GetSource(OPTION_SOURCE).(*Config)
}
