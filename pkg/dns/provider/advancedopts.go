// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type AdvancedConfig struct {
	BatchSize  int
	MaxRetries int
}

////////////////////////////////////////////////////////////////////////////////

type AdvancedOptions struct {
	BatchSize    int
	MaxRetries   int
	BlockedZones []string
}

var AdvancedOptionsDefaults = AdvancedOptions{
	BatchSize:    50,
	MaxRetries:   7,
	BlockedZones: []string{},
}

func (this *AdvancedOptions) AddOptionsToSet(set config.OptionSet) {
	set.AddIntOption(&this.BatchSize, OPT_ADVANCED_BATCH_SIZE, "", 50, "batch size for change requests (currently only used for aws-route53)")
	set.AddIntOption(&this.MaxRetries, OPT_ADVANCED_MAX_RETRIES, "", 7, "maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)")
	set.AddStringArrayOption(&this.BlockedZones, OPT_ADVANCED_BLOCKED_ZONE, "", []string{}, "Blocks a zone given in the format `zone-id` from a provider as if the zone is not existing.")
}

func (c *AdvancedOptions) GetAdvancedConfig() AdvancedConfig {
	return AdvancedConfig{BatchSize: c.BatchSize, MaxRetries: c.MaxRetries}
}

func (c *AdvancedOptions) GetBlockedZones() utils.StringSet {
	return utils.NewStringSet(c.BlockedZones...)
}

// configuration helpers

func (c AdvancedOptions) SetBatchSize(batchSize int) AdvancedOptions {
	c.BatchSize = batchSize
	return c
}

func (c AdvancedOptions) SetMaxRetries(maxRetries int) AdvancedOptions {
	c.MaxRetries = maxRetries
	return c
}

////////////////////////////////////////////////////////////////////////////////

func (c AdvancedConfig) String() string {
	return fmt.Sprintf("BatchSize: %d, MaxRetries: %d", c.BatchSize, c.MaxRetries)
}
