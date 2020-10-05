/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package utils

import (
	"time"
)

type RateLimiter interface {
	Succeeded()
	Failed()
	RateLimit() time.Duration
}

type defaultRateLimiter struct {
	min     time.Duration
	max     time.Duration
	minincr time.Duration
	factor  float64

	rate time.Duration
}

func NewDefaultRateLimiter(min, max time.Duration) RateLimiter {
	if min <= 0 {
		min = time.Second
	}
	if max <= min {
		max = min * 20
	}
	return &defaultRateLimiter{
		min:     min,
		max:     max,
		minincr: min / 10,
		factor:  1.1,
	}
}

func NewRateLimiter(min, max, minincr time.Duration, factor float64) RateLimiter {
	if min <= 0 {
		min = time.Second
	}
	if max <= min {
		max = min * 20
	}
	if minincr <= 0 {
		minincr = min / 20
	}
	if factor < 0.0 {
		factor = -factor
	}
	if factor < 1.0 {
		factor += 1.0
	}
	return &defaultRateLimiter{
		min:     min,
		max:     max,
		minincr: minincr,
		factor:  factor,
	}
}

func (this *defaultRateLimiter) RateLimit() time.Duration {
	return this.rate
}

func (this *defaultRateLimiter) Succeeded() {
	this.rate = 0
}

func (this *defaultRateLimiter) Failed() {
	if this.rate == 0 {
		this.rate = this.min
	} else {
		if this.rate < this.max {
			n := this.factor * float64(this.rate)
			if time.Duration(n-float64(this.rate)) < this.minincr {
				this.rate = this.rate + this.minincr
			} else {
				this.rate = time.Duration(n)
			}
		} else {
			this.rate = this.max
		}
	}
}
