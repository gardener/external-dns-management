/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
	"k8s.io/client-go/util/flowcontrol"
)

type RateLimiterConfigProvider interface {
	GetRateLimiterConfig() *RateLimiterConfig
}

func AddRawRateLimiterConfigToOptionSet(set config.OptionSet, raw *RawRateLimiterConfig, defaults RawRateLimiterConfig) {
	set.AddBoolOption(&raw.Enabled, OPT_RATELIMITER_ENABLED, "", defaults.Enabled, "enables rate limiter for DNS provider requests")
	set.AddIntOption(&raw.QPS, OPT_RATELIMITER_QPS, "", defaults.QPS, "maximum requests/queries per second")
	set.AddIntOption(&raw.Burst, OPT_RATELIMITER_BURST, "", defaults.Burst, "number of burst requests for rate limiter")
}

func (c *RawRateLimiterConfig) GetRateLimiterConfig() *RateLimiterConfig {
	if !c.Enabled {
		return nil
	}
	return &RateLimiterConfig{QPS: float32(c.QPS), Burst: c.Burst}
}

func (c *RateLimiterConfig) NewRateLimiter() (flowcontrol.RateLimiter, error) {
	if c == nil {
		return AlwaysRateLimiter(), nil
	}

	if c.QPS < 0.01 || c.QPS > 1e4 {
		return nil, fmt.Errorf("invalid qps value %f", c.QPS)
	}
	if c.Burst < 0 || c.Burst > 10000 {
		return nil, fmt.Errorf("invalid burst value %d", c.Burst)
	}

	return flowcontrol.NewTokenBucketRateLimiter(c.QPS, c.Burst), nil
}

func AlwaysRateLimiter() flowcontrol.RateLimiter {
	return flowcontrol.NewFakeAlwaysRateLimiter()
}
