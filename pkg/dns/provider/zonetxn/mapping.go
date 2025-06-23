// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package zonetxn

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/aws/mapping"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/utils"
)

func ApplySpec(set *dns.DNSSet, providerType string, spec utils.TargetSpec) *dns.DNSSet {
	targets := spec.Targets()
	if providerType == "aws-route53" {
		targets = mapping.MapTargets(spec.Targets())
	}
	for _, t := range targets {
		set.Sets.AddRecord(t.GetRecordType(), t.GetHostName(), t.GetTTL())
	}
	return set
}
