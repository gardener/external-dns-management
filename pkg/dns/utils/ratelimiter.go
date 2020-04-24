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
)

type RateLimiter struct {
	min     time.Duration
	max     time.Duration
	minincr time.Duration

	rate time.Duration
}

func NewRateLimiter(min, max, minincr time.Duration) *RateLimiter {
	if min <= 0 {
		min = time.Second
	}
	if max <= min {
		max = min * 20
	}
	if minincr < min/10 {
		minincr = min / 10
	}
	return &RateLimiter{
		min: min,
		max: max,
	}
}

func (this *RateLimiter) RateLimit() time.Duration {
	return this.rate
}

func (this *RateLimiter) Succeeded() {
	this.rate = 0
}

func (this *RateLimiter) Failed() {
	if this.rate == 0 {
		this.rate = this.min
	} else {
		if this.rate < this.max {
			this.rate = time.Duration(1.1*float64(this.rate)) + time.Second
		}
	}
}
