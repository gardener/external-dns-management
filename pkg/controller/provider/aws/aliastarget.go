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
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/gardener/external-dns-management/pkg/dns"
	"strings"
)

var (
	// original code: https://github.com/kubernetes-incubator/external-dns/blob/master/provider/aws.go
	// see: https://docs.aws.amazon.com/general/latest/gr/rande.html#elb_region
	canonicalHostedZones = map[string]string{
		// Application Load Balancers and Classic Load Balancers
		"us-east-2.elb.amazonaws.com":         "Z3AADJGX6KTTL2",
		"us-east-1.elb.amazonaws.com":         "Z35SXDOTRQ7X7K",
		"us-west-1.elb.amazonaws.com":         "Z368ELLRRE2KJ0",
		"us-west-2.elb.amazonaws.com":         "Z1H1FL5HABSF5",
		"ca-central-1.elb.amazonaws.com":      "ZQSVJUPU6J1EY",
		"ap-east-1.elb.amazonaws.com":         "Z3DQVH9N71FHZ0",
		"ap-south-1.elb.amazonaws.com":        "ZP97RAFLXTNZK",
		"ap-northeast-2.elb.amazonaws.com":    "ZWKZPGTI48KDX",
		"ap-northeast-3.elb.amazonaws.com":    "Z5LXEXXYW11ES",
		"ap-southeast-1.elb.amazonaws.com":    "Z1LMS91P8CMLE5",
		"ap-southeast-2.elb.amazonaws.com":    "Z1GM3OXH4ZPM65",
		"ap-northeast-1.elb.amazonaws.com":    "Z14GRHDCWA56QT",
		"eu-central-1.elb.amazonaws.com":      "Z215JYRZR1TBD5",
		"eu-west-1.elb.amazonaws.com":         "Z32O12XQLNTSW2",
		"eu-west-2.elb.amazonaws.com":         "ZHURV8PSTC4K8",
		"eu-west-3.elb.amazonaws.com":         "Z3Q77PNBQS71R4",
		"eu-north-1.elb.amazonaws.com":        "Z23TAZ6LKFMNIO",
		"sa-east-1.elb.amazonaws.com":         "Z2P70J7HTTTPLU",
		"cn-north-1.elb.amazonaws.com.cn":     "Z3BX2TMKNYI13Y",
		"cn-northwest-1.elb.amazonaws.com.cn": "Z3BX2TMKNYI13Y",
		// Network Load Balancers
		"elb.us-east-2.amazonaws.com":         "ZLMOA37VPKANP",
		"elb.us-east-1.amazonaws.com":         "Z26RNL4JYFTOTI",
		"elb.us-west-1.amazonaws.com":         "Z24FKFUX50B4VW",
		"elb.us-west-2.amazonaws.com":         "Z18D5FSROUN65G",
		"elb.ca-central-1.amazonaws.com":      "Z2EPGBW3API2WT",
		"elb.ap-east-1.amazonaws.com":         "Z12Y7K3UBGUAD1",
		"elb.ap-south-1.amazonaws.com":        "ZVDDRBQ08TROA",
		"elb.ap-northeast-2.amazonaws.com":    "ZIBE1TIR4HY56",
		"elb.ap-southeast-1.amazonaws.com":    "ZKVM4W9LS7TM",
		"elb.ap-southeast-2.amazonaws.com":    "ZCT6FZBF4DROD",
		"elb.ap-northeast-1.amazonaws.com":    "Z31USIVHYNEOWT",
		"elb.eu-central-1.amazonaws.com":      "Z3F0SRJ5LGBH90",
		"elb.eu-west-1.amazonaws.com":         "Z2IFOLAFXWLO4F",
		"elb.eu-west-2.amazonaws.com":         "ZD4D7Y8KGAS4G",
		"elb.eu-west-3.amazonaws.com":         "Z1CMS0P5QUZ6D5",
		"elb.eu-north-1.amazonaws.com":        "Z1UDT6IFJ4EJM",
		"elb.sa-east-1.amazonaws.com":         "ZTK26PT1VY4CU",
		"elb.cn-north-1.amazonaws.com.cn":     "Z3QFB96KMJ7ED6",
		"elb.cn-northwest-1.amazonaws.com.cn": "ZQEIKTCZ8352D",
	}
)

func isAliasTarget(r *route53.ResourceRecordSet) bool {
	return aws.StringValue(r.Type) == route53.RRTypeA && r.AliasTarget != nil
}

// buildRecordSetForAliasTarget transforms an A alias target to a CNAME dns.RecordSet
func buildRecordSetForAliasTarget(r *route53.ResourceRecordSet) *dns.RecordSet {
	rs := dns.NewRecordSet(dns.RS_CNAME, 0, nil)
	rs.IgnoreTTL = true // alias target has no settable TTL
	rs.Add(&dns.Record{Value: dns.NormalizeHostname(aws.StringValue(r.AliasTarget.DNSName))})
	return rs
}

// canConvertToAliasTarget determines if a given hostname belongs to an AWS load balancer.
// Returns nil otherwise.
func canConvertToAliasTarget(rset *dns.RecordSet) bool {
	return getAliasTargetForAWSLoadBalancer(rset) != nil
}

// getAliasTargetForAWSLoadBalancer determines if a given hostname belongs to an AWS load balancer
// and creates an AliasTarget. Returns nil otherwise.
func getAliasTargetForAWSLoadBalancer(rset *dns.RecordSet) *route53.AliasTarget {
	if rset.Type == dns.RS_CNAME && len(rset.Records) == 1 {
		target := dns.NormalizeHostname(rset.Records[0].Value)
		hostedZone := canonicalHostedZone(target)
		if hostedZone != "" {
			return &route53.AliasTarget{
				DNSName:              aws.String(target),
				HostedZoneId:         aws.String(hostedZone),
				EvaluateTargetHealth: aws.Bool(true),
			}
		}
	}

	return nil
}

func buildResourceRecordSetForAliasTarget(name string, rset *dns.RecordSet) *route53.ResourceRecordSet {
	return &route53.ResourceRecordSet{
		Name:        aws.String(name),
		Type:        aws.String(route53.RRTypeA),
		AliasTarget: getAliasTargetForAWSLoadBalancer(rset),
	}
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
