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

package google

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/gardener/external-dns-management/pkg/dns"
	googledns "google.golang.org/api/dns/v1"
)

const routingPolicyMaxIndices = 5

type googleRoutingPolicyData struct {
	index  int
	weight int64
}

type dnsname = string
type dnstype = string

type routingPolicyChanges map[dnsname]map[dnstype]*googledns.ResourceRecordSet

type rrsetGetterFunc func(name, typ string) (*googledns.ResourceRecordSet, error)

var _deleted_marker_ = &googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{}

func (c routingPolicyChanges) addChange(set *googledns.ResourceRecordSet, add bool) {
	perType := c[set.Name]
	if perType == nil {
		perType = map[dnstype]*googledns.ResourceRecordSet{}
	}
	current := perType[set.Type]
	if current == nil {
		current = &googledns.ResourceRecordSet{
			Name: set.Name,
			RoutingPolicy: &googledns.RRSetRoutingPolicy{
				Wrr: &googledns.RRSetRoutingPolicyWrrPolicy{},
			},
			Ttl:  set.Ttl,
			Type: set.Type,
		}
	}
	index := len(set.RoutingPolicy.Wrr.Items) - 1
	for len(current.RoutingPolicy.Wrr.Items) <= index {
		current.RoutingPolicy.Wrr.Items = append(current.RoutingPolicy.Wrr.Items, nil)
	}
	if add {
		current.RoutingPolicy.Wrr.Items[index] = set.RoutingPolicy.Wrr.Items[index]
	} else {
		current.RoutingPolicy.Wrr.Items[index] = _deleted_marker_
	}
	perType[set.Type] = current
	c[set.Name] = perType
}

func (c routingPolicyChanges) calcDeletionsAndAdditions(rrsetGetter rrsetGetterFunc) (deletions []*googledns.ResourceRecordSet, additions []*googledns.ResourceRecordSet, err error) {
	for name, perType := range c {
		for typ, rrset := range perType {
			old, err2 := rrsetGetter(name, typ)
			if err2 == nil {
				deletions = append(deletions, old)
				for i, item := range old.RoutingPolicy.Wrr.Items {
					if i < len(rrset.RoutingPolicy.Wrr.Items) {
						if rrset.RoutingPolicy.Wrr.Items[i] == nil {
							rrset.RoutingPolicy.Wrr.Items[i] = item
						}
					} else {
						rrset.RoutingPolicy.Wrr.Items = append(rrset.RoutingPolicy.Wrr.Items, item)
					}
				}
			} else if !isNotFound(err2) {
				err = err2
				return
			}

			max := len(rrset.RoutingPolicy.Wrr.Items) - 1
			for i := len(rrset.RoutingPolicy.Wrr.Items) - 1; i >= 0; i-- {
				if rrset.RoutingPolicy.Wrr.Items[i] == _deleted_marker_ {
					if max == i {
						rrset.RoutingPolicy.Wrr.Items = rrset.RoutingPolicy.Wrr.Items[:i]
						max = i - 1
					} else {
						rrset.RoutingPolicy.Wrr.Items[i] = createWrrPlaceHolderItem(typ)
					}
				} else if rrset.RoutingPolicy.Wrr.Items[i] == nil {
					rrset.RoutingPolicy.Wrr.Items[i] = createWrrPlaceHolderItem(typ)
				} else if max == i && isWrrPlaceHolderItem(typ, rrset.RoutingPolicy.Wrr.Items[i]) {
					rrset.RoutingPolicy.Wrr.Items = rrset.RoutingPolicy.Wrr.Items[:i]
					max = i - 1
				}
			}
			if len(rrset.RoutingPolicy.Wrr.Items) > 0 {
				additions = append(additions, rrset)
			}
		}
	}
	err = nil
	return
}

func extractRoutingPolicy(set *dns.DNSSet) (*googleRoutingPolicyData, error) {
	if set.Name.SetIdentifier == "" && set.RoutingPolicy == nil {
		return nil, nil
	}
	if set.Name.SetIdentifier == "" {
		return nil, fmt.Errorf("missing set identifier")
	}
	if set.RoutingPolicy == nil {
		return nil, fmt.Errorf("missing routing policy")
	}
	index, err := strconv.Atoi(set.Name.SetIdentifier)
	if index < 0 || index >= routingPolicyMaxIndices || err != nil {
		return nil, fmt.Errorf("For %s, the setIdentifier must be an number >= 0 and < %d, but got: %s", TYPE_CODE, routingPolicyMaxIndices, set.Name.SetIdentifier)
	}
	var keys []string
	switch set.RoutingPolicy.Type {
	case dns.RoutingPolicyWeighted:
		keys = []string{"weight"}
	default:
		return nil, fmt.Errorf("unsupported routing policy: %s", set.RoutingPolicy.Type)
	}

	if err := set.RoutingPolicy.CheckParameterKeys(keys); err != nil {
		return nil, err
	}

	var weight int64
	for key, value := range set.RoutingPolicy.Parameters {
		switch key {
		case "weight":
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil || v < 0 {
				return nil, fmt.Errorf("invalid value for spec.routingPolicy.parameters.weight: %s (only non-negative integers are allowed)", value)
			}
			weight = v
		}
	}

	return &googleRoutingPolicyData{
		index:  index,
		weight: weight,
	}, nil
}

func mapPolicyRecordSet(rrset *googledns.ResourceRecordSet, data *googleRoutingPolicyData) *googledns.ResourceRecordSet {
	if data == nil {
		return rrset
	}

	items := make([]*googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem, data.index+1)
	items[data.index] = &googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{
		Rrdatas: rrset.Rrdatas,
		Weight:  float64(data.weight),
	}

	return &googledns.ResourceRecordSet{
		Name: rrset.Name,
		RoutingPolicy: &googledns.RRSetRoutingPolicy{
			Wrr: &googledns.RRSetRoutingPolicyWrrPolicy{
				Items: items,
			},
		},
		Ttl:  rrset.Ttl,
		Type: rrset.Type,
	}
}

func describeRoutingPolicy(rrset *googledns.ResourceRecordSet) string {
	if rrset.RoutingPolicy == nil || rrset.RoutingPolicy.Wrr == nil {
		return ""
	}
	buf := new(bytes.Buffer)
	for i, item := range rrset.RoutingPolicy.Wrr.Items {
		if !isWrrPlaceHolderItem(rrset.Type, item) {
			buf.WriteString(fmt.Sprintf("[%d]%.1f:%s;", i, item.Weight, strings.Join(item.Rrdatas, ",")))
		}
	}
	return buf.String()
}

func createWrrPlaceHolderItem(typ string) *googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem {
	return &googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{
		Rrdatas: []string{rrDefaultValue(typ)},
		Weight:  0,
	}
}

func isWrrPlaceHolderItem(typ string, item *googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem) bool {
	return item.Weight == 0 && len(item.Rrdatas) == 1 && item.Rrdatas[0] == rrDefaultValue(typ)
}

func rrDefaultValue(typ string) string {
	switch typ {
	case dns.RS_TXT:
		return "\"__dummy__\""
	case dns.RS_A:
		// use dummy documentation IP address
		return "233.252.0.1"
	case dns.RS_CNAME:
		return "dummy.dummy.dummy.com."
	case dns.RS_AAAA:
		// use dummy documentation IP address
		return "2001:db8::1"
	default:
		return typ + "?"
	}
}
