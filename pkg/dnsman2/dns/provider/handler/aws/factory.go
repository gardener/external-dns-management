// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/aws/mapping"
)

// ProviderType is the type identifier for the AWS Route53 DNS handler.
const ProviderType = "aws-route53"

const (
	defaultBatchSize  = 50
	defaultMaxRetries = 7
)

// RegisterTo registers the AWS Route53 DNS handler to the given registry.
func RegisterTo(registry *provider.DNSHandlerRegistry) {
	registry.Register(
		ProviderType,
		NewHandler,
		&config.RateLimiterOptions{
			Enabled: true,
			QPS:     9,
			Burst:   10,
		},
		&targetsMapper{},
	)
}

type targetsMapper struct{}

func (m *targetsMapper) MapTargets(targets []dns.Target) []dns.Target {
	return mapping.MapTargets(targets)
}
