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

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"k8s.io/client-go/util/flowcontrol"
)

func NewRateLimiterConfig(c controller.Interface) *RateLimiterConfig {
	enabled, _ := c.GetBoolOption(OPT_RATELIMITER_ENABLED)
	if !enabled {
		return nil
	}
	qps, _ := c.GetIntOption(OPT_RATELIMITER_QPS)
	burst, _ := c.GetIntOption(OPT_RATELIMITER_BURST)
	return &RateLimiterConfig{QPS: float32(qps), Burst: burst}
}

func (c *RateLimiterConfig) NewRateLimiter() (flowcontrol.RateLimiter, error) {
	if c == nil {
		return flowcontrol.NewFakeAlwaysRateLimiter(), nil
	}

	if c.QPS < 0.01 || c.QPS > 1e4 {
		return nil, fmt.Errorf("invalid qps value %f", c.QPS)
	}
	if c.Burst < 0 || c.Burst > 10000 {
		return nil, fmt.Errorf("invalid burst value %d", c.Burst)
	}

	return flowcontrol.NewTokenBucketRateLimiter(c.QPS, c.Burst), nil
}
