/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package utils

import (
	"time"

	"go.uber.org/atomic"
)

type RateLimiter struct {
	min time.Duration
	max time.Duration

	rate atomic.Duration
}

func NewRateLimiter(min, max time.Duration) *RateLimiter {
	if min <= 0 {
		min = time.Second
	}
	if max <= min {
		max = min * 20
	}
	return &RateLimiter{
		min: min,
		max: max,
	}
}

func (this *RateLimiter) RateLimit() time.Duration {
	rate := this.rate.Load()
	if rate == 0 {
		rate = this.min
	}
	return rate
}

func (this *RateLimiter) Succeeded() {
	this.rate.Store(0)
}

func (this *RateLimiter) Failed() {
	newRate := this.min
	rate := this.rate.Load()
	if rate > 0 {
		newRate = time.Duration(1.1*float64(rate)) + time.Second
		if rate > this.max {
			newRate = this.max
		}
	}
	this.rate.Store(newRate)
}
