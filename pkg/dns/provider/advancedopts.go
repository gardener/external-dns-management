/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package provider

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/config"
)

type AdvancedConfig struct {
	BatchSize  int
	MaxRetries int
}

////////////////////////////////////////////////////////////////////////////////

type AdvancedOptions struct {
	BatchSize  int
	MaxRetries int
}

var AdvancedOptionsDefaults = AdvancedOptions{
	BatchSize:  50,
	MaxRetries: 7,
}

func (this *AdvancedOptions) AddOptionsToSet(set config.OptionSet) {
	set.AddIntOption(&this.BatchSize, OPT_ADVANCED_BATCH_SIZE, "", 50, "batch size for change requests (currently only used for aws-route53)")
	set.AddIntOption(&this.MaxRetries, OPT_ADVANCED_MAX_RETRIES, "", 7, "maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)")
}

func (c *AdvancedOptions) GetAdvancedConfig() AdvancedConfig {
	return AdvancedConfig{BatchSize: c.BatchSize, MaxRetries: c.MaxRetries}
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
