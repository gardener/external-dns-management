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
	"net"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/gardener/external-dns-management/pkg/controller/provider/aws/data"
	"github.com/gardener/external-dns-management/pkg/dns"
)

var canonicalHostedZones = data.CanonicalHostedZones()

func isAliasTarget(r *route53.ResourceRecordSet) bool {
	return (aws.StringValue(r.Type) == route53.RRTypeA || aws.StringValue(r.Type) == route53.RRTypeAaaa) && r.AliasTarget != nil
}

// buildRecordSetFromAliasTarget transforms an A alias target to a CNAME dns.RecordSet
func buildRecordSetFromAliasTarget(r *route53.ResourceRecordSet) *dns.RecordSet {
	rs := dns.NewRecordSet(dns.RS_ALIAS, 0, nil)
	rs.IgnoreTTL = true // alias target has no settable TTL
	rs.Add(&dns.Record{Value: dns.NormalizeHostname(aws.StringValue(r.AliasTarget.DNSName))})
	return rs
}

func buildResourceRecordSetsForAliasTarget(name dns.DNSSetName, policy *dns.RoutingPolicy, policyContext *routingPolicyContext, rset *dns.RecordSet) ([]*route53.ResourceRecordSet, error) {
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

	ips, err := net.LookupIP(dns.AlignHostname(rset.Records[0].Value))
	hasIPv4 := err != nil || len(ips) == 0 // assume it has IPv4 addresses if lookup fails temporarily
	hasIPv6 := err != nil || len(ips) == 0 // assume it has IPv6 addresses if lookup fails temporarily
	for _, ip := range ips {
		if ip.To4() != nil {
			hasIPv4 = true
		} else {
			hasIPv6 = true
		}
	}
	var types []string
	if hasIPv4 {
		types = append(types, route53.RRTypeA)
	}
	if hasIPv6 {
		types = append(types, route53.RRTypeAaaa)
	}
	var rrsets []*route53.ResourceRecordSet
	for _, t := range types {
		rrset := &route53.ResourceRecordSet{
			Name:        aws.String(name.DNSName),
			Type:        aws.String(t),
			AliasTarget: aliasTarget,
		}
		if err := policyContext.addRoutingPolicy(rrset, name, policy); err != nil {
			return nil, err
		}
		rrsets = append(rrsets, rrset)
	}
	return rrsets, nil
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
