/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package config

import (
	"time"

	"github.com/gardener/controller-manager-library/pkg/config"
	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/config"
)

const OPTION_SOURCE = "controllers"

type Config struct {
	Controllers        string
	OmitLease          bool
	LeaseName          string
	LeaseDuration      time.Duration
	LeaseRenewDeadline time.Duration
	LeaseRetryPeriod   time.Duration

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
	cfg.AddDurationOption(&cfg.LeaseDuration, "lease-duration", "", 15*time.Second, "lease duration")
	cfg.AddDurationOption(&cfg.LeaseRenewDeadline, "lease-renew-deadline", "", 10*time.Second, "lease renew deadline")
	cfg.AddDurationOption(&cfg.LeaseRetryPeriod, "lease-retry-period", "", 2*time.Second, "lease retry period")
	return cfg
}

func (this *Config) Evaluate() error {
	return this.OptionSet.Evaluate()
}

func GetConfig(cfg *areacfg.Config) *Config {
	return cfg.GetSource(OPTION_SOURCE).(*Config)
}
