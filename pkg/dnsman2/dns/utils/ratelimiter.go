// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"time"

	"go.uber.org/atomic"
)

// RateLimiter provides a mechanism to limit the rate of operations with exponential backoff.
type RateLimiter struct {
	min time.Duration
	max time.Duration

	rate atomic.Duration
}

// NewRateLimiter creates a new RateLimiter with the given minimum and maximum durations.
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

// RateLimit returns the current rate limit duration.
func (this *RateLimiter) RateLimit() time.Duration {
	rate := this.rate.Load()
	if rate == 0 {
		rate = this.min
	}
	return rate
}

// Succeeded resets the rate limiter after a successful operation.
func (this *RateLimiter) Succeeded() {
	this.rate.Store(0)
}

// Failed increases the rate limit duration after a failed operation.
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
