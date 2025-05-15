// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package alicloud

import (
	"fmt"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

func checkValidRoutingPolicy(req *provider.ChangeRequest) error {
	if req.Addition != nil {
		if err := checkRoutingPolicyForDNSSet(req.Addition); err != nil {
			return err
		}
	}
	if req.Deletion != nil {
		if err := checkRoutingPolicyForDNSSet(req.Deletion); err != nil {
			return err
		}
	}
	return nil
}

func checkRoutingPolicyForDNSSet(set *dns.DNSSet) error {
	if set.Name.SetIdentifier == "" && set.RoutingPolicy == nil {
		return nil
	}
	if set.Name.SetIdentifier == "" {
		return fmt.Errorf("missing set identifier")
	}
	if set.RoutingPolicy == nil {
		return fmt.Errorf("missing routing policy")
	}
	if set.RoutingPolicy.Type != dns.RoutingPolicyWeighted {
		return fmt.Errorf("unsupported routing policy")
	}
	for t, _ := range set.Sets {
		switch t {
		case dns.RS_A, dns.RS_AAAA:
			// ok
		default:
			return fmt.Errorf("weighted routing policy only supported for A and AAAA records")
		}
	}
	return nil
}
