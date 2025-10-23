// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"

	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
)

// NewRateLimiter creates a new rate limiter based on the given options.
func NewRateLimiter(options *config.RateLimiterOptions) (flowcontrol.RateLimiter, error) {
	if options == nil || !options.Enabled {
		return flowcontrol.NewFakeAlwaysRateLimiter(), nil
	}

	qps := options.QPS
	if qps < 0.01 || qps > 1e4 {
		return nil, fmt.Errorf("invalid qps value %f", qps)
	}
	if options.Burst < 0 || options.Burst > 10000 {
		return nil, fmt.Errorf("invalid burst value %d", options.Burst)
	}

	return flowcontrol.NewTokenBucketRateLimiter(qps, options.Burst), nil
}
