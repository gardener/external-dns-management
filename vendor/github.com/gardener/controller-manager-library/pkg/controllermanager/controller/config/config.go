/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package config

import (
	"github.com/gardener/controller-manager-library/pkg/config"
	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/lease"
)

const OPTION_SOURCE = "controllers"

type Config struct {
	Controllers string
	Lease       lease.Config

	config.OptionSet
}

var _ config.OptionSource = (*Config)(nil)

func NewConfig() *Config {
	cfg := &Config{
		OptionSet: config.NewSharedOptionSet(OPTION_SOURCE, ""),
	}
	cfg.AddStringOption(&cfg.Controllers, "controllers", "c", "all", "comma separated list of controllers to start (<name>,<group>,all)")
	cfg.Lease.AddOptionsToSet(cfg.OptionSet)
	return cfg
}

func (this *Config) Evaluate() error {
	return this.OptionSet.Evaluate()
}

func GetConfig(cfg *areacfg.Config) *Config {
	return cfg.GetSource(OPTION_SOURCE).(*Config)
}
