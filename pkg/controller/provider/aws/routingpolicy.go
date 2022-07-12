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
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/gardener/external-dns-management/pkg/dns"
)

func addRoutingPolicy(rrset *route53.ResourceRecordSet, name dns.RecordSetName, routingPolicy *dns.RoutingPolicy) error {
	if name.SetIdentifier == "" && routingPolicy == nil {
		return nil
	}
	if name.SetIdentifier == "" {
		return fmt.Errorf("routing policy set, but missing set identifier")
	}
	if routingPolicy == nil {
		return fmt.Errorf("set identifier set, but routing policy missing")
	}

	var keys []string
	switch routingPolicy.Type {
	case dns.RoutingPolicyWeighted:
		keys = []string{"weight"}
	default:
		return fmt.Errorf("unsupported routing policy type %s", routingPolicy.Type)
	}

	if err := routingPolicy.CheckParameterKeys(keys); err != nil {
		return err
	}

	rrset.SetIdentifier = aws.String(name.SetIdentifier)
	for key, value := range routingPolicy.Parameters {
		switch key {
		case "weight":
			v, err := strconv.ParseInt(value, 0, 64)
			if err != nil || v < 0 {
				return fmt.Errorf("invalid value for spec.routingPolicy.parameters.weight: %s", value)
			}
			rrset.Weight = aws.Int64(v)
		}
	}

	return nil
}

func extractRoutingPolicy(rrset *route53.ResourceRecordSet) *dns.RoutingPolicy {
	if rrset.SetIdentifier == nil {
		return nil
	}

	if rrset.Weight != nil {
		return dns.NewRoutingPolicy(dns.RoutingPolicyWeighted, "weight", strconv.FormatInt(*rrset.Weight, 10))
	}
	// ignore unsupported routing policy
	return nil
}
