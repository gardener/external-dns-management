// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package alicloud

import (
	"fmt"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

func checkValidRoutingPolicy(name dns.DNSSetName, req *provider.ChangeRequestUpdate) error {
	if req.Old != nil {
		if err := checkRoutingPolicyForDNSSet(name, req.Old); err != nil {
			return err
		}
	}
	if req.New != nil {
		if err := checkRoutingPolicyForDNSSet(name, req.New); err != nil {
			return err
		}
	}
	return nil
}

func checkRoutingPolicyForDNSSet(name dns.DNSSetName, rs *dns.RecordSet) error {
	if name.SetIdentifier == "" && rs.RoutingPolicy == nil {
		return nil
	}
	if name.SetIdentifier == "" {
		return fmt.Errorf("missing set identifier")
	}
	if rs.RoutingPolicy == nil {
		return fmt.Errorf("missing routing policy")
	}
	if rs.RoutingPolicy.Type != dns.RoutingPolicyWeighted {
		return fmt.Errorf("unsupported routing policy")
	}
	switch rs.Type {
	case dns.TypeA, dns.TypeAAAA:
		// ok
	default:
		return fmt.Errorf("weighted routing policy only supported for A and AAAA records")
	}
	return nil
}
