// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/aws/mapping"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// buildRecordSetFromAliasTarget transforms an A or AAAA alias target to a ALIAS_A or ALIAS_AAAA dns.RecordSet.
// Otherwise returns nil.
func buildRecordSetFromAliasTarget(r route53types.ResourceRecordSet) *dns.RecordSet {
	if r.AliasTarget == nil {
		return nil
	}
	var rtype dns.RecordType
	switch r.Type {
	case route53types.RRTypeA:
		rtype = dns.TypeAWS_ALIAS_A
	case route53types.RRTypeAaaa:
		rtype = dns.TypeAWS_ALIAS_AAAA
	default:
		return nil
	}
	rs := dns.NewRecordSet(rtype, 0, nil)
	rs.IgnoreTTL = true // alias target has no settable TTL
	rs.Add(&dns.Record{Value: dns.NormalizeDomainName(aws.ToString(r.AliasTarget.DNSName))})
	return rs
}

// buildResourceRecordSetForAliasTarget transforms a ALIAS_A or ALIAS_AAAA dns.RecordSet to a route53 resource record set.
// Otherwise returns nil.
func buildResourceRecordSetForAliasTarget(ctx context.Context, name dns.DNSSetName, policy *dns.RoutingPolicy, policyContext *routingPolicyContext, rset *dns.RecordSet) (*route53types.ResourceRecordSet, error) {
	var rtype route53types.RRType
	switch rset.Type {
	case dns.TypeAWS_ALIAS_A:
		rtype = route53types.RRTypeA
	case dns.TypeAWS_ALIAS_AAAA:
		rtype = route53types.RRTypeAaaa
	default:
		return nil, nil
	}

	target := dns.NormalizeDomainName(rset.Records[0].Value)
	hostedZone := mapping.CanonicalHostedZone(target)
	if hostedZone == "" {
		return nil, fmt.Errorf("Corrupted alias record set")
	}
	aliasTarget := &route53types.AliasTarget{
		DNSName:              aws.String(target),
		HostedZoneId:         aws.String(hostedZone),
		EvaluateTargetHealth: true,
	}

	rrset := &route53types.ResourceRecordSet{
		Name:        aws.String(name.DNSName),
		Type:        rtype,
		AliasTarget: aliasTarget,
	}
	if err := policyContext.addRoutingPolicy(ctx, rrset, name, policy); err != nil {
		return nil, err
	}
	return rrset, nil
}
