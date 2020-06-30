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
	"github.com/gardener/controller-manager-library/pkg/config"
	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/config"
)

const OPTION_SOURCE = "controllers"

type Config struct {
	Controllers string
	OmitLease   bool
	LeaseName   string

	config.OptionSet
}

var _ config.OptionSource = (*Config)(nil)

func NewConfig() *Config {
	cfg := &Config{
		OptionSet: config.NewDefaultOptionSet(OPTION_SOURCE, ""),
	}
	cfg.AddStringOption(&cfg.Controllers, "controllers", "c", "all", "comma separated list of controllers to start (<name>,<group>,all)")
	cfg.AddStringOption(&cfg.LeaseName, "lease-name", "", "", "name for lease object")
	cfg.AddBoolOption(&cfg.OmitLease, "omit-lease", "", false, "omit lease for development")
	return cfg
}

func (this *Config) Evaluate() error {
	return this.OptionSet.Evaluate()
}

func GetConfig(cfg *areacfg.Config) *Config {
	return cfg.GetSource(OPTION_SOURCE).(*Config)
}
