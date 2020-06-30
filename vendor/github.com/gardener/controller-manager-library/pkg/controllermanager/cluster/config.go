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

package cluster

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/config"
)

const OPTION_SOURCE = "cluster"

const SUBOPTION_ID = "id"
const SUBOPTION_DISABLE_DEPLOY_CRDS = "disable-deploy-crds"

type Config struct {
	Definition
	KubeConfig string
	ClusterId  string
	OmitCRDs   bool

	config.OptionSet
}

var _ config.OptionSource = (*Config)(nil)

func configTargetKey(def Definition) string {
	return "cluster." + def.Name()
}

func NewConfig(def Definition) *Config {
	cfg := &Config{
		Definition: def,
		OptionSet:  config.NewDefaultOptionSet(configTargetKey(def), def.ConfigOptionName()),
	}
	cfg.AddStringOption(&cfg.ClusterId, SUBOPTION_ID, "", "", fmt.Sprintf("id for cluster %s", def.Name()))
	cfg.AddBoolOption(&cfg.OmitCRDs, SUBOPTION_DISABLE_DEPLOY_CRDS, "", false, fmt.Sprintf("disable deployment of required crds for cluster %s", def.Name()))
	callExtensions(func(e Extension) error { e.ExtendConfig(def, cfg); return nil })
	return cfg
}

func (this *Config) AddOptionsToSet(set config.OptionSet) {
	if this.ConfigOptionName() != "" {
		set.AddStringOption(&this.KubeConfig, this.ConfigOptionName(), "", "", this.Description())
	}
	this.OptionSet.AddOptionsToSet(set)
}
