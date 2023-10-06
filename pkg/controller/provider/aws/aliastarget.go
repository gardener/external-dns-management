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
	"github.com/gardener/external-dns-management/pkg/dns"
)

// original code: https://github.com/kubernetes-sigs/external-dns/blob/master/provider/aws/aws.go
// see: https://docs.aws.amazon.com/general/latest/gr/elb.html
var canonicalHostedZones = map[string]string{
	// Application Load Balancers and Classic Load Balancers
	"us-east-2.elb.amazonaws.com":         "Z3AADJGX6KTTL2",
	"us-east-1.elb.amazonaws.com":         "Z35SXDOTRQ7X7K",
	"us-west-1.elb.amazonaws.com":         "Z368ELLRRE2KJ0",
	"us-west-2.elb.amazonaws.com":         "Z1H1FL5HABSF5",
	"ca-central-1.elb.amazonaws.com":      "ZQSVJUPU6J1EY",
	"ap-east-1.elb.amazonaws.com":         "Z3DQVH9N71FHZ0",
	"ap-south-1.elb.amazonaws.com":        "ZP97RAFLXTNZK",
	"ap-south-2.elb.amazonaws.com":        "Z0173938T07WNTVAEPZN",
	"ap-northeast-2.elb.amazonaws.com":    "ZWKZPGTI48KDX",
	"ap-northeast-3.elb.amazonaws.com":    "Z5LXEXXYW11ES",
	"ap-southeast-1.elb.amazonaws.com":    "Z1LMS91P8CMLE5",
	"ap-southeast-2.elb.amazonaws.com":    "Z1GM3OXH4ZPM65",
	"ap-southeast-3.elb.amazonaws.com":    "Z08888821HLRG5A9ZRTER",
	"ap-southeast-4.elb.amazonaws.com":    "Z09517862IB2WZLPXG76F",
	"ap-northeast-1.elb.amazonaws.com":    "Z14GRHDCWA56QT",
	"eu-central-1.elb.amazonaws.com":      "Z215JYRZR1TBD5",
	"eu-central-2.elb.amazonaws.com":      "Z06391101F2ZOEP8P5EB3",
	"eu-west-1.elb.amazonaws.com":         "Z32O12XQLNTSW2",
	"eu-west-2.elb.amazonaws.com":         "ZHURV8PSTC4K8",
	"eu-west-3.elb.amazonaws.com":         "Z3Q77PNBQS71R4",
	"eu-north-1.elb.amazonaws.com":        "Z23TAZ6LKFMNIO",
	"eu-south-1.elb.amazonaws.com":        "Z3ULH7SSC9OV64",
	"eu-south-2.elb.amazonaws.com":        "Z0956581394HF5D5LXGAP",
	"sa-east-1.elb.amazonaws.com":         "Z2P70J7HTTTPLU",
	"cn-north-1.elb.amazonaws.com.cn":     "Z1GDH35T77C1KE",
	"cn-northwest-1.elb.amazonaws.com.cn": "ZM7IZAIOVVDZF",
	"us-gov-west-1.elb.amazonaws.com":     "Z33AYJ8TM3BH4J",
	"us-gov-east-1.elb.amazonaws.com":     "Z166TLBEWOO7G0",
	"me-central-1.elb.amazonaws.com":      "Z08230872XQRWHG2XF6I",
	"me-south-1.elb.amazonaws.com":        "ZS929ML54UICD",
	"af-south-1.elb.amazonaws.com":        "Z268VQBMOI5EKX",
	// Network Load Balancers
	"elb.us-east-2.amazonaws.com":         "ZLMOA37VPKANP",
	"elb.us-east-1.amazonaws.com":         "Z26RNL4JYFTOTI",
	"elb.us-west-1.amazonaws.com":         "Z24FKFUX50B4VW",
	"elb.us-west-2.amazonaws.com":         "Z18D5FSROUN65G",
	"elb.ca-central-1.amazonaws.com":      "Z2EPGBW3API2WT",
	"elb.ap-east-1.amazonaws.com":         "Z12Y7K3UBGUAD1",
	"elb.ap-south-1.amazonaws.com":        "ZVDDRBQ08TROA",
	"elb.ap-south-2.amazonaws.com":        "Z0711778386UTO08407HT",
	"elb.ap-northeast-3.amazonaws.com":    "Z1GWIQ4HH19I5X",
	"elb.ap-northeast-2.amazonaws.com":    "ZIBE1TIR4HY56",
	"elb.ap-southeast-1.amazonaws.com":    "ZKVM4W9LS7TM",
	"elb.ap-southeast-2.amazonaws.com":    "ZCT6FZBF4DROD",
	"elb.ap-southeast-3.amazonaws.com":    "Z01971771FYVNCOVWJU1G",
	"elb.ap-southeast-4.amazonaws.com":    "Z01156963G8MIIL7X90IV",
	"elb.ap-northeast-1.amazonaws.com":    "Z31USIVHYNEOWT",
	"elb.eu-central-1.amazonaws.com":      "Z3F0SRJ5LGBH90",
	"elb.eu-central-2.amazonaws.com":      "Z02239872DOALSIDCX66S",
	"elb.eu-west-1.amazonaws.com":         "Z2IFOLAFXWLO4F",
	"elb.eu-west-2.amazonaws.com":         "ZD4D7Y8KGAS4G",
	"elb.eu-west-3.amazonaws.com":         "Z1CMS0P5QUZ6D5",
	"elb.eu-north-1.amazonaws.com":        "Z1UDT6IFJ4EJM",
	"elb.eu-south-1.amazonaws.com":        "Z23146JA1KNAFP",
	"elb.eu-south-2.amazonaws.com":        "Z1011216NVTVYADP1SSV",
	"elb.il-central-1.amazonaws.com":      "Z0313266YDI6ZRHTGQY4",
	"elb.sa-east-1.amazonaws.com":         "ZTK26PT1VY4CU",
	"elb.cn-north-1.amazonaws.com.cn":     "Z3QFB96KMJ7ED6",
	"elb.cn-northwest-1.amazonaws.com.cn": "ZQEIKTCZ8352D",
	"elb.us-gov-west-1.amazonaws.com":     "ZMG1MZ2THAWF1",
	"elb.us-gov-east-1.amazonaws.com":     "Z1ZSMQQ6Q24QQ8",
	"elb.me-central-1.amazonaws.com":      "Z00282643NTTLPANJJG2P",
	"elb.me-south-1.amazonaws.com":        "Z3QSRYVP46NYYV",
	"elb.af-south-1.amazonaws.com":        "Z203XCE67M25HM",
	// Global Accelerator
	"awsglobalaccelerator.com": "Z2BJ6XQ5FK7U4H",
	// Cloudfront and AWS API Gateway edge-optimized endpoints
	"cloudfront.net": "Z2FDTNDATAQYW2",
	// VPC Endpoint (PrivateLink)
	"eu-west-2.vpce.amazonaws.com":      "Z7K1066E3PUKB",
	"us-east-2.vpce.amazonaws.com":      "ZC8PG0KIFKBRI",
	"af-south-1.vpce.amazonaws.com":     "Z09302161J80N9A7UTP7U",
	"ap-east-1.vpce.amazonaws.com":      "Z2LIHJ7PKBEMWN",
	"ap-northeast-1.vpce.amazonaws.com": "Z2E726K9Y6RL4W",
	"ap-northeast-2.vpce.amazonaws.com": "Z27UANNT0PRK1T",
	"ap-northeast-3.vpce.amazonaws.com": "Z376B5OMM2JZL2",
	"ap-south-1.vpce.amazonaws.com":     "Z2KVTB3ZLFM7JR",
	"ap-south-2.vpce.amazonaws.com":     "Z0952991RWSF5AHIQDIY",
	"ap-southeast-1.vpce.amazonaws.com": "Z18LLCSTV4NVNL",
	"ap-southeast-2.vpce.amazonaws.com": "ZDK2GCRPAFKGO",
	"ap-southeast-3.vpce.amazonaws.com": "Z03881013RZ9BYYZO8N5W",
	"ap-southeast-4.vpce.amazonaws.com": "Z07508191CO1RNBX3X3AU",
	"ca-central-1.vpce.amazonaws.com":   "ZRCXCF510Y6P9",
	"eu-central-1.vpce.amazonaws.com":   "Z273ZU8SZ5RJPC",
	"eu-central-2.vpce.amazonaws.com":   "Z045369019J4FUQ4S272E",
	"eu-north-1.vpce.amazonaws.com":     "Z3OWWK6JFDEDGC",
	"eu-south-1.vpce.amazonaws.com":     "Z2A5FDNRLY7KZG",
	"eu-south-2.vpce.amazonaws.com":     "Z014396544HENR57XQCJ",
	"eu-west-1.vpce.amazonaws.com":      "Z38GZ743OKFT7T",
	"eu-west-3.vpce.amazonaws.com":      "Z1DWHTMFP0WECP",
	"me-central-1.vpce.amazonaws.com":   "Z07122992YCEUCB9A9570",
	"me-south-1.vpce.amazonaws.com":     "Z3B95P3VBGEQGY",
	"sa-east-1.vpce.amazonaws.com":      "Z2LXUWEVLCVZIB",
	"us-east-1.vpce.amazonaws.com":      "Z7HUB22UULQXV",
	"us-gov-east-1.vpce.amazonaws.com":  "Z2MU5TEIGO9WXB",
	"us-gov-west-1.vpce.amazonaws.com":  "Z12529ZODG2B6H",
	"us-west-1.vpce.amazonaws.com":      "Z12I86A8N7VCZO",
	"us-west-2.vpce.amazonaws.com":      "Z1YSA3EXCYUU9Z",
	// AWS API Gateway (Regional endpoints)
	// See: https://docs.aws.amazon.com/general/latest/gr/apigateway.html
	"execute-api.us-east-2.amazonaws.com":      "ZOJJZC49E0EPZ",
	"execute-api.us-east-1.amazonaws.com":      "Z1UJRXOUMOOFQ8",
	"execute-api.us-west-1.amazonaws.com":      "Z2MUQ32089INYE",
	"execute-api.us-west-2.amazonaws.com":      "Z2OJLYMUO9EFXC",
	"execute-api.af-south-1.amazonaws.com":     "Z2DHW2332DAMTN",
	"execute-api.ap-east-1.amazonaws.com":      "Z3FD1VL90ND7K5",
	"execute-api.ap-south-1.amazonaws.com":     "Z3VO1THU9YC4UR",
	"execute-api.ap-northeast-2.amazonaws.com": "Z20JF4UZKIW1U8",
	"execute-api.ap-southeast-1.amazonaws.com": "ZL327KTPIQFUL",
	"execute-api.ap-southeast-2.amazonaws.com": "Z2RPCDW04V8134",
	"execute-api.ap-northeast-1.amazonaws.com": "Z1YSHQZHG15GKL",
	"execute-api.ca-central-1.amazonaws.com":   "Z19DQILCV0OWEC",
	"execute-api.eu-central-1.amazonaws.com":   "Z1U9ULNL0V5AJ3",
	"execute-api.eu-west-1.amazonaws.com":      "ZLY8HYME6SFDD",
	"execute-api.eu-west-2.amazonaws.com":      "ZJ5UAJN8Y3Z2Q",
	"execute-api.eu-south-1.amazonaws.com":     "Z3BT4WSQ9TDYZV",
	"execute-api.eu-west-3.amazonaws.com":      "Z3KY65QIEKYHQQ",
	"execute-api.eu-south-2.amazonaws.com":     "Z02499852UI5HEQ5JVWX3",
	"execute-api.eu-north-1.amazonaws.com":     "Z3UWIKFBOOGXPP",
	"execute-api.me-south-1.amazonaws.com":     "Z20ZBPC0SS8806",
	"execute-api.me-central-1.amazonaws.com":   "Z08780021BKYYY8U0YHTV",
	"execute-api.sa-east-1.amazonaws.com":      "ZCMLWB8V5SYIT",
	"execute-api.us-gov-east-1.amazonaws.com":  "Z3SE9ATJYCRCZJ",
	"execute-api.us-gov-west-1.amazonaws.com":  "Z1K6XKP9SAGWDV",
}

func isAliasTarget(r *route53.ResourceRecordSet) bool {
	return aws.StringValue(r.Type) == route53.RRTypeA && r.AliasTarget != nil
}

// buildRecordSetFromAliasTarget transforms an A alias target to a CNAME dns.RecordSet
func buildRecordSetFromAliasTarget(r *route53.ResourceRecordSet) *dns.RecordSet {
	rs := dns.NewRecordSet(dns.RS_ALIAS, 0, nil)
	rs.IgnoreTTL = true // alias target has no settable TTL
	rs.Add(&dns.Record{Value: dns.NormalizeHostname(aws.StringValue(r.AliasTarget.DNSName))})
	return rs
}

func buildResourceRecordSetForAliasTarget(name dns.DNSSetName, policy *dns.RoutingPolicy, policyContext *routingPolicyContext, rset *dns.RecordSet) (*route53.ResourceRecordSet, error) {
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
		Type:        aws.String(route53.RRTypeA),
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
