// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/config"
	"k8s.io/client-go/util/flowcontrol"
)

type RateLimiterConfig struct {
	QPS   float32
	Burst int
}

////////////////////////////////////////////////////////////////////////////////

type RateLimiterOptions struct {
	Enabled bool
	QPS     int
	Burst   int
}

var RateLimiterOptionDefaults = RateLimiterOptions{
	Enabled: true,
	QPS:     10,
	Burst:   20,
}

func (this *RateLimiterOptions) AddOptionsToSet(set config.OptionSet) {
	set.AddBoolOption(&this.Enabled, OPT_RATELIMITER_ENABLED, "", this.Enabled, "enables rate limiter for DNS provider requests")
	set.AddIntOption(&this.QPS, OPT_RATELIMITER_QPS, "", this.QPS, "maximum requests/queries per second")
	set.AddIntOption(&this.Burst, OPT_RATELIMITER_BURST, "", this.Burst, "number of burst requests for rate limiter")
}

func (c *RateLimiterOptions) GetRateLimiterConfig() *RateLimiterConfig {
	if !c.Enabled {
		return nil
	}
	return &RateLimiterConfig{QPS: float32(c.QPS), Burst: c.Burst}
}

// configuration helpers

func (c RateLimiterOptions) SetQPS(qps int) RateLimiterOptions {
	c.QPS = qps
	return c
}

func (c RateLimiterOptions) SetBurst(burst int) RateLimiterOptions {
	c.Burst = burst
	return c
}

func (c RateLimiterOptions) SetEnabled(enabled bool) RateLimiterOptions {
	c.Enabled = enabled
	return c
}

////////////////////////////////////////////////////////////////////////////////

func (c *RateLimiterConfig) String() string {
	return fmt.Sprintf("QPS: %f, Burst: %d", c.QPS, c.Burst)
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
