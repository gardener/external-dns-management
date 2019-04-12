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

package provider

import (
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/access"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

func filterByZones(domains utils.StringSet, zones DNSHostedZones) (result utils.StringSet, err error) {
	result = utils.StringSet{}
	for d := range domains {
	_zones:
		for _, z := range zones {
			if dnsutils.Match(d, z.Domain()) {
				for _, sub := range z.ForwardedDomains() {
					if dnsutils.Match(d, sub) {
						continue _zones
					}
				}
				result.Add(d)
				break
			}
		}
		if !result.Contains(d) {
			err = fmt.Errorf("domain %q not in hosted domains", d)
		}
	}
	return result, err
}

func copyZones(src map[string]*dnsHostedZone) dnsHostedZones {
	dst := dnsHostedZones{}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func CheckAccess(object resources.Object, used resources.Object) error {
	var err error

	owners := object.GetOwners()
	if len(owners) > 0 {
		for o := range owners {
			ok, msg, aerr := access.Allowed(o, "use", used.ClusterKey())
			if !ok {
				if aerr != nil {
					err = fmt.Errorf("%s: %s: %s", o, msg, err)
				} else {
					err = fmt.Errorf("%s: %s", o, msg)
				}
			}
		}
	} else {
		o := object.ClusterKey()
		ok, msg, aerr := access.Allowed(o, "use", used.ClusterKey())
		if !ok {
			if aerr != nil {
				err = fmt.Errorf("%s: %s: %s", used.ClusterKey(), msg, err)
			} else {
				err = fmt.Errorf("%s: %s", used.ClusterKey(), msg)
			}
		}
	}
	return err
}

func ErrorValue(err error) string {
	if err == nil {
		return "<no error>"
	}
	return err.Error()
}
