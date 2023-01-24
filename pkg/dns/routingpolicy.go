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

package dns

import (
	"fmt"
)

const (
	// RoutingPolicyWeighted is a weighted routing policy (supported for AWS Route 53 and Google CloudDNS)
	RoutingPolicyWeighted = "weighted"
	// RoutingPolicyLatency is a latency based routing policy (supported for AWS Route 53)
	RoutingPolicyLatency = "latency"
	// RoutingPolicyGeoLocation is a geolocation based routing policy (supported for AWS Route 53 and Google CloudDNS)
	RoutingPolicyGeoLocation = "geolocation"
	// RoutingPolicyIPBased is an IP based routing policy (supported for AWS Route 53)
	RoutingPolicyIPBased = "ip-based"
	// RoutingPolicyFailover is failover routing policy (supported for AWS Route 53)
	RoutingPolicyFailover = "failover"
)

type RoutingPolicy struct {
	Type       string
	Parameters map[string]string
}

func NewRoutingPolicy(typ string, keyvalues ...string) *RoutingPolicy {
	policy := &RoutingPolicy{Type: typ, Parameters: map[string]string{}}
	for i := 0; i < len(keyvalues)-1; i += 2 {
		policy.Parameters[keyvalues[i]] = keyvalues[i+1]
	}
	return policy
}

func (p *RoutingPolicy) Clone() *RoutingPolicy {
	if p == nil {
		return nil
	}
	copy := &RoutingPolicy{Type: p.Type, Parameters: map[string]string{}}
	for k, v := range p.Parameters {
		copy.Parameters[k] = v
	}
	return copy
}

func (p *RoutingPolicy) CheckParameterKeys(keys, optionalKeys []string) error {
	for _, k := range keys {
		if _, ok := p.Parameters[k]; !ok {
			return fmt.Errorf("Missing parameter key %s", k)
		}
	}
	if len(keys) != len(p.Parameters) {
	outer:
		for k := range p.Parameters {
			for _, k2 := range keys {
				if k == k2 {
					continue outer
				}
			}
			for _, k2 := range optionalKeys {
				if k == k2 {
					continue outer
				}
			}
			return fmt.Errorf("Unsupported parameter key %s", k)
		}
	}
	return nil
}
