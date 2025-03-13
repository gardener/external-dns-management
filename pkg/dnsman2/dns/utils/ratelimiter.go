// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
