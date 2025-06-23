// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package google

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	googledns "google.golang.org/api/dns/v1"

	"github.com/gardener/external-dns-management/pkg/dns"
)

const routingPolicyMaxIndices = 5

const (
	keyWeight   = "weight"
	keyLocation = "location"
)

type googleRoutingPolicyData struct {
	index    int
	weight   *int64
	location *string
}

type (
	dnsname = string
	dnstype = string
)

type routingPolicyChanges map[dnsname]map[dnstype]*googledns.ResourceRecordSet

type rrsetGetterFunc func(name, typ string) (*googledns.ResourceRecordSet, error)

var _deleted_marker_wrr_ = &googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{}

func (c routingPolicyChanges) addChange(set *googledns.ResourceRecordSet, add bool) {
	perType := c[set.Name]
	if perType == nil {
		perType = map[dnstype]*googledns.ResourceRecordSet{}
	}
	current := perType[set.Type]
	routingPolicy := routingPolicyFromRRS(current)
	if current == nil {
		routingPolicy = routingPolicyFromRRS(set)
		current = &googledns.ResourceRecordSet{
			Name:          set.Name,
			RoutingPolicy: &googledns.RRSetRoutingPolicy{},
			Ttl:           set.Ttl,
			Type:          set.Type,
		}
		switch routingPolicy {
		case dns.RoutingPolicyWeighted:
			current.RoutingPolicy.Wrr = &googledns.RRSetRoutingPolicyWrrPolicy{}
		case dns.RoutingPolicyGeoLocation:
			current.RoutingPolicy.Geo = &googledns.RRSetRoutingPolicyGeoPolicy{}
		}
	}
	switch routingPolicy {
	case dns.RoutingPolicyWeighted:
		index := len(set.RoutingPolicy.Wrr.Items) - 1
		for len(current.RoutingPolicy.Wrr.Items) <= index {
			current.RoutingPolicy.Wrr.Items = append(current.RoutingPolicy.Wrr.Items, nil)
		}
		if add {
			current.RoutingPolicy.Wrr.Items[index] = set.RoutingPolicy.Wrr.Items[index]
		} else {
			current.RoutingPolicy.Wrr.Items[index] = _deleted_marker_wrr_
		}
	case dns.RoutingPolicyGeoLocation:
		item := set.RoutingPolicy.Geo.Items[0]
		// kind is misused as delete marker
		if !add {
			item.Kind = "delete"
		}
		current.RoutingPolicy.Geo.Items = append(current.RoutingPolicy.Geo.Items, item)
	}
	perType[set.Type] = current
	c[set.Name] = perType
}

func routingPolicyFromRRS(set *googledns.ResourceRecordSet) string {
	if set == nil || set.RoutingPolicy == nil {
		return ""
	}
	if set.RoutingPolicy.Wrr != nil {
		return dns.RoutingPolicyWeighted
	}
	if set.RoutingPolicy.Geo != nil {
		return dns.RoutingPolicyGeoLocation
	}
	return ""
}

func (c routingPolicyChanges) calcDeletionsAndAdditions(rrsetGetter rrsetGetterFunc) (deletions []*googledns.ResourceRecordSet, additions []*googledns.ResourceRecordSet, err error) {
	for name, perType := range c {
		for typ, rrset := range perType {
			routingPolicy := routingPolicyFromRRS(rrset)
			switch routingPolicy {
			case dns.RoutingPolicyWeighted:
				deletions, additions, err = c.calcDeletionsAndAdditionsWrr(rrsetGetter, name, typ, rrset, deletions, additions)
			case dns.RoutingPolicyGeoLocation:
				deletions, additions, err = c.calcDeletionsAndAdditionsGeo(rrsetGetter, name, typ, rrset, deletions, additions)
			}
			if err != nil {
				return
			}
		}
	}
	return
}

func (c routingPolicyChanges) calcDeletionsAndAdditionsWrr(
	rrsetGetter rrsetGetterFunc, name dnsname, typ dnstype,
	rrset *googledns.ResourceRecordSet,
	deletions, additions []*googledns.ResourceRecordSet,
) ([]*googledns.ResourceRecordSet, []*googledns.ResourceRecordSet, error) {
	old, err := rrsetGetter(name, typ)
	if err == nil {
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
	} else if !isNotFound(err) {
		return deletions, additions, err
	}

	max := len(rrset.RoutingPolicy.Wrr.Items) - 1
	for i := len(rrset.RoutingPolicy.Wrr.Items) - 1; i >= 0; i-- {
		if rrset.RoutingPolicy.Wrr.Items[i] == _deleted_marker_wrr_ {
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

	return deletions, additions, nil
}

func (c routingPolicyChanges) calcDeletionsAndAdditionsGeo(
	rrsetGetter rrsetGetterFunc, name dnsname, typ dnstype,
	rrset *googledns.ResourceRecordSet,
	deletions, additions []*googledns.ResourceRecordSet,
) ([]*googledns.ResourceRecordSet, []*googledns.ResourceRecordSet, error) {
	old, err := rrsetGetter(name, typ)
	if err == nil {
		deletions = append(deletions, old)
		changedLocations := map[string]struct{}{}
		for _, item := range rrset.RoutingPolicy.Geo.Items {
			changedLocations[item.Location] = struct{}{}
		}
		if old.RoutingPolicy != nil && old.RoutingPolicy.Geo != nil {
			for _, item := range old.RoutingPolicy.Geo.Items {
				if _, found := changedLocations[item.Location]; !found {
					rrset.RoutingPolicy.Geo.Items = append(rrset.RoutingPolicy.Geo.Items, item)
				}
			}
		}
	} else if !isNotFound(err) {
		return deletions, additions, err
	}

	for i := len(rrset.RoutingPolicy.Geo.Items); i > 0; i-- {
		if rrset.RoutingPolicy.Geo.Items[i-1].Kind == "delete" {
			rrset.RoutingPolicy.Geo.Items = append(rrset.RoutingPolicy.Geo.Items[:i-1], rrset.RoutingPolicy.Geo.Items[i:]...)
		}
	}
	if len(rrset.RoutingPolicy.Geo.Items) > 0 {
		additions = append(additions, rrset)
	}

	return deletions, additions, nil
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
	var index int
	var err error
	var keys []string
	switch set.RoutingPolicy.Type {
	case dns.RoutingPolicyWeighted:
		keys = []string{keyWeight}
		index, err = strconv.Atoi(set.Name.SetIdentifier)
		if index < 0 || index >= routingPolicyMaxIndices || err != nil {
			return nil, fmt.Errorf("for %s with weighted routing policy, the setIdentifier must be an number >= 0 and < %d, but got: %s", TYPE_CODE, routingPolicyMaxIndices, set.Name.SetIdentifier)
		}
	case dns.RoutingPolicyGeoLocation:
		keys = []string{keyLocation}
		if set.Name.SetIdentifier != set.RoutingPolicy.Parameters[keyLocation] {
			return nil, fmt.Errorf("for %s with geolocation routing policy, the setIdentifier must be identical to the location, but: %s != %s", TYPE_CODE, set.Name.SetIdentifier, set.RoutingPolicy.Parameters[keyLocation])
		}
	default:
		return nil, fmt.Errorf("unsupported routing policy: %s", set.RoutingPolicy.Type)
	}

	if err := set.RoutingPolicy.CheckParameterKeys(keys, nil); err != nil {
		return nil, err
	}

	data := &googleRoutingPolicyData{index: index}
	for key, value := range set.RoutingPolicy.Parameters {
		switch key {
		case keyWeight:
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil || v < 0 {
				return nil, fmt.Errorf("invalid value for spec.routingPolicy.parameters.weight: %s (only non-negative integers are allowed)", value)
			}
			data.weight = &v
		case keyLocation:
			vv := value
			data.location = &vv
		}
	}

	return data, nil
}

func mapPolicyRecordSet(rrset *googledns.ResourceRecordSet, data *googleRoutingPolicyData) *googledns.ResourceRecordSet {
	if data == nil {
		return rrset
	}

	if data.weight != nil {
		return mapPolicyRecordSetWeighted(rrset, data)
	}
	if data.location != nil {
		return mapPolicyRecordSetGeo(rrset, data)
	}
	return rrset
}

func mapPolicyRecordSetWeighted(rrset *googledns.ResourceRecordSet, data *googleRoutingPolicyData) *googledns.ResourceRecordSet {
	items := make([]*googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem, data.index+1)
	items[data.index] = &googledns.RRSetRoutingPolicyWrrPolicyWrrPolicyItem{
		Rrdatas: rrset.Rrdatas,
		Weight:  float64(*data.weight),
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

func mapPolicyRecordSetGeo(rrset *googledns.ResourceRecordSet, data *googleRoutingPolicyData) *googledns.ResourceRecordSet {
	items := make([]*googledns.RRSetRoutingPolicyGeoPolicyGeoPolicyItem, data.index+1)
	items[data.index] = &googledns.RRSetRoutingPolicyGeoPolicyGeoPolicyItem{
		Rrdatas:  rrset.Rrdatas,
		Location: *data.location,
	}

	return &googledns.ResourceRecordSet{
		Name: rrset.Name,
		RoutingPolicy: &googledns.RRSetRoutingPolicy{
			Geo: &googledns.RRSetRoutingPolicyGeoPolicy{
				Items: items,
			},
		},
		Ttl:  rrset.Ttl,
		Type: rrset.Type,
	}
}

func describeRoutingPolicy(rrset *googledns.ResourceRecordSet) string {
	if rrset.RoutingPolicy == nil {
		return ""
	}

	if rrset.RoutingPolicy.Wrr != nil {
		buf := new(bytes.Buffer)
		for i, item := range rrset.RoutingPolicy.Wrr.Items {
			if !isWrrPlaceHolderItem(rrset.Type, item) {
				fmt.Fprintf(buf, "wrr:[%d]%.1f:%s;", i, item.Weight, strings.Join(item.Rrdatas, ","))
			}
		}
		return buf.String()
	}

	if rrset.RoutingPolicy.Geo != nil {
		buf := new(bytes.Buffer)
		for _, item := range rrset.RoutingPolicy.Geo.Items {
			fmt.Fprintf(buf, "geo:%s:%s;", item.Location, strings.Join(item.Rrdatas, ","))
		}
		return buf.String()
	}

	return ""
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
