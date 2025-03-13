// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"fmt"
)

type RoutingPolicyType string

const (
	// RoutingPolicyWeighted is a weighted routing policy (supported for AWS Route 53 and Google CloudDNS)
	RoutingPolicyWeighted RoutingPolicyType = "weighted"
	// RoutingPolicyLatency is a latency based routing policy (supported for AWS Route 53)
	RoutingPolicyLatency RoutingPolicyType = "latency"
	// RoutingPolicyGeoLocation is a geolocation based routing policy (supported for AWS Route 53 and Google CloudDNS)
	RoutingPolicyGeoLocation RoutingPolicyType = "geolocation"
	// RoutingPolicyIPBased is an IP based routing policy (supported for AWS Route 53)
	RoutingPolicyIPBased RoutingPolicyType = "ip-based"
	// RoutingPolicyFailover is failover routing policy (supported for AWS Route 53)
	RoutingPolicyFailover RoutingPolicyType = "failover"
)

type RoutingPolicy struct {
	Type       RoutingPolicyType
	Parameters map[string]string
}

func NewRoutingPolicy(typ RoutingPolicyType, keyvalues ...string) *RoutingPolicy {
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
	copy := &RoutingPolicy{Type: p.Type}
	if len(p.Parameters) > 0 {
		copy.Parameters = map[string]string{}
		for k, v := range p.Parameters {
			copy.Parameters[k] = v
		}
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
