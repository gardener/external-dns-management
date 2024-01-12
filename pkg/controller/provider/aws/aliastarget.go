/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package aws

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/gardener/external-dns-management/pkg/controller/provider/aws/data"
	"github.com/gardener/external-dns-management/pkg/dns"
)

var canonicalHostedZones = data.CanonicalHostedZones()

// buildRecordSetFromAliasTarget transforms an A or AAAA alias target to a ALIAS_A or ALIAS_AAAA dns.RecordSet.
// Otherwise returns nil.
func buildRecordSetFromAliasTarget(r *route53.ResourceRecordSet) *dns.RecordSet {
	if r.AliasTarget == nil {
		return nil
	}
	var rtype string
	switch aws.StringValue(r.Type) {
	case route53.RRTypeA:
		rtype = dns.RS_ALIAS_A
	case route53.RRTypeAaaa:
		rtype = dns.RS_ALIAS_AAAA
	default:
		return nil
	}
	rs := dns.NewRecordSet(rtype, 0, nil)
	rs.IgnoreTTL = true // alias target has no settable TTL
	rs.Add(&dns.Record{Value: dns.NormalizeHostname(aws.StringValue(r.AliasTarget.DNSName))})
	return rs
}

// buildResourceRecordSetForAliasTarget transforms a ALIAS_A or ALIAS_AAAA dns.RecordSet to a route53 resource record set.
// Otherwise returns nil.
func buildResourceRecordSetForAliasTarget(name dns.DNSSetName, policy *dns.RoutingPolicy, policyContext *routingPolicyContext, rset *dns.RecordSet) (*route53.ResourceRecordSet, error) {
	var rtype string
	switch rset.Type {
	case dns.RS_ALIAS_A:
		rtype = route53.RRTypeA
	case dns.RS_ALIAS_AAAA:
		rtype = route53.RRTypeAaaa
	default:
		return nil, nil
	}

	target := dns.NormalizeHostname(rset.Records[0].Value)
	hostedZone := canonicalHostedZone(target)
	if hostedZone == "" {
		return nil, fmt.Errorf("Corrupted alias record set")
	}
	aliasTarget := &route53.AliasTarget{
		DNSName:              aws.String(target),
		HostedZoneId:         aws.String(hostedZone),
		EvaluateTargetHealth: aws.Bool(true),
	}

	rrset := &route53.ResourceRecordSet{
		Name:        aws.String(name.DNSName),
		Type:        aws.String(rtype),
		AliasTarget: aliasTarget,
	}
	if err := policyContext.addRoutingPolicy(rrset, name, policy); err != nil {
		return nil, err
	}
	return rrset, nil
}

// canonicalHostedZone returns the matching canonical zone for a given hostname.
func canonicalHostedZone(hostname string) string {
	for suffix, zone := range canonicalHostedZones {
		if strings.HasSuffix(hostname, suffix) {
			return zone
		}
	}

	return ""
}
