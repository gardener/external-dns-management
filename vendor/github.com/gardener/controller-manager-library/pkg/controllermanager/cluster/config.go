/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cluster

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

const OPTION_SOURCE = "cluster"

const SUBOPTION_ID = "id"
const SUBOPTION_MIGIDS = "migration-ids"
const SUBOPTION_DISABLE_DEPLOY_CRDS = "disable-deploy-crds"

type Config struct {
	Definition
	KubeConfig   string
	ClusterId    string
	MigrationIds utils.StringSet
	OmitCRDs     bool

	migrationIds string

	config.OptionSet

	set config.OptionSet
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
	cfg.AddStringOption(&cfg.migrationIds, SUBOPTION_MIGIDS, "", "", fmt.Sprintf("migration id for cluster %s", def.Name()))
	cfg.AddBoolOption(&cfg.OmitCRDs, SUBOPTION_DISABLE_DEPLOY_CRDS, "", false, fmt.Sprintf("disable deployment of required crds for cluster %s", def.Name()))
	callExtensions(func(e Extension) error { e.ExtendConfig(def, cfg); return nil })
	return cfg
}

func (this *Config) AddOptionsToSet(set config.OptionSet) {
	if this.ConfigOptionName() != "" {
		set.AddStringOption(&this.KubeConfig, this.ConfigOptionName(), "", "", this.Description())
	}
	this.OptionSet.AddOptionsToSet(set)
	this.set = set
}

func (this *Config) Evaluate() error {
	this.MigrationIds = utils.StringSet{}
	if this.migrationIds != "" {
		this.MigrationIds.AddAllSplittedSelected(this.migrationIds, utils.NonEmptyStringElement)
	}
	return nil
}

func (this *Config) IsConfigured() bool {
	return this.ClusterId != "" || this.set.GetOption(this.ConfigOptionName()).Changed()
}
