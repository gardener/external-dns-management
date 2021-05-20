/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package lease

import (
	"time"

	"github.com/gardener/controller-manager-library/pkg/config"
)

type Config struct {
	OmitLease          bool
	LeaseName          string
	LeaseDuration      time.Duration
	LeaseRenewDeadline time.Duration
	LeaseRetryPeriod   time.Duration
}

func (this *Config) AddOptionsToSet(set config.OptionSet) {
	set.AddStringOption(&this.LeaseName, "lease-name", "", "", "name for lease object")
	set.AddBoolOption(&this.OmitLease, "omit-lease", "", false, "omit lease for development")
	set.AddDurationOption(&this.LeaseDuration, "lease-duration", "", 15*time.Second, "lease duration")
	set.AddDurationOption(&this.LeaseRenewDeadline, "lease-renew-deadline", "", 10*time.Second, "lease renew deadline")
	set.AddDurationOption(&this.LeaseRetryPeriod, "lease-retry-period", "", 2*time.Second, "lease retry period")
}
