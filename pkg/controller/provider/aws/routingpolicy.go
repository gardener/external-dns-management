// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

	"github.com/gardener/external-dns-management/pkg/dns"
)

const (
	keyWeight                      = "weight"
	keyRegion                      = "region"
	keyLocation                    = "location"
	keyCollection                  = "collection"
	keyFailoverRecordType          = "failoverRecordType"
	keyDisableEvaluateTargetHealth = "disableEvaluateTargetHealth"
	keyHealthCheckID               = "healthCheckID"

	// refreshGeoLocationPeriod is the interval to reload the geolocation names and codes
	refreshGeoLocationPeriod = 24 * time.Hour
	// refreshGeoLocationPeriodNotFound is the interval to reload the geolocation names and codes if name has not been found
	refreshGeoLocationPeriodNotFound = 30 * time.Minute
	// refreshGeoLocationPeriod is the interval to reload the CIDR collections
	refreshCIDRCollectionsPeriod = 1 * time.Hour
	// refreshCIDRCollectionsPeriodNotFound is the interval to reload the CIDR collections if collection or location name has not been found
	refreshCIDRCollectionsPeriodNotFound = 15 * time.Minute
)

func newRoutingPolicyContext(r53 route53.Client) *routingPolicyContext {
	return &routingPolicyContext{
		r53:                            r53,
		cachedCIDRCollectionNameToID:   map[string]string{},
		cachedCIDRCollectionIDToName:   map[string]string{},
		cachedCIDRCollectionIDToBlocks: map[string]cidrBlockMap{},
		lastCIDRCollectionBlocksUpdate: map[string]time.Time{},
	}
}

type routingPolicyContext struct {
	sync.Mutex
	r53                             route53.Client
	cachedGeoLocationNameToLocation map[string]*route53types.GeoLocation
	cachedGeoLocationCodeToName     map[string]string
	lastGeoLocationListUpdate       time.Time

	cachedCIDRCollectionNameToID   map[string]string
	cachedCIDRCollectionIDToName   map[string]string
	cachedCIDRCollectionIDToBlocks map[string]cidrBlockMap
	lastCIDRCollectionUpdate       time.Time
	lastCIDRCollectionBlocksUpdate map[string]time.Time
}

type cidrBlockMap struct {
	locationToCIDRBlock map[string]string
	cidrBlockToLocation map[string]string
}

func (r *routingPolicyContext) lookupGeoLocation(ctx context.Context, name string) (*route53types.GeoLocation, error) {
	r.Lock()
	defer r.Unlock()

	if err := r.refreshGeoLocationList(ctx, refreshGeoLocationPeriod); err != nil {
		return nil, err
	}

	location := r.cachedGeoLocationNameToLocation[name]
	if location == nil {
		// if not found refresh may be retried
		if err := r.refreshGeoLocationList(ctx, refreshGeoLocationPeriodNotFound); err != nil {
			return nil, err
		}
		location = r.cachedGeoLocationNameToLocation[name]
	}

	if location == nil {
		return nil, fmt.Errorf("location %s not found", name)
	}
	return location, nil
}

func (r *routingPolicyContext) geoLocationName(ctx context.Context, geoLocation *route53types.GeoLocation) (string, error) {
	r.Lock()
	defer r.Unlock()

	if geoLocation == nil {
		return "", fmt.Errorf("geoLocation is nil")
	}

	if err := r.refreshGeoLocationList(ctx, refreshGeoLocationPeriod); err != nil {
		return "", err
	}

	code := codeFromGeoLocation(geoLocation)
	name := r.cachedGeoLocationCodeToName[code]
	if name == "" {
		// if not found refresh may be retried
		if err := r.refreshGeoLocationList(ctx, refreshGeoLocationPeriodNotFound); err != nil {
			return "", err
		}
		name = r.cachedGeoLocationCodeToName[code]
	}

	if name == "" {
		return "", fmt.Errorf("location code '%s' not found", code)
	}
	return name, nil
}

func (r *routingPolicyContext) refreshGeoLocationList(ctx context.Context, period time.Duration) error {
	if r.lastGeoLocationListUpdate.Add(period).After(time.Now()) {
		return nil
	}

	output, err := r.r53.ListGeoLocations(ctx, &route53.ListGeoLocationsInput{})
	if err != nil {
		r.lastGeoLocationListUpdate = time.Now().Add(-period / 2)
		return fmt.Errorf("listing geo-locations failed: %s", err)
	}

	r.lastGeoLocationListUpdate = time.Now()
	r.cachedGeoLocationCodeToName = map[string]string{}
	r.cachedGeoLocationNameToLocation = map[string]*route53types.GeoLocation{}

	for _, details := range output.GeoLocationDetailsList {
		var name, altName string
		if details.SubdivisionName != nil {
			name = aws.ToString(details.SubdivisionName)
			altName = fmt.Sprintf("country=%s,subdivision=%s", aws.ToString(details.CountryCode), aws.ToString(details.SubdivisionCode))
		} else if details.CountryName != nil {
			name = aws.ToString(details.CountryName)
			altName = fmt.Sprintf("country=%s", aws.ToString(details.CountryCode))
		} else if details.ContinentName != nil {
			name = aws.ToString(details.ContinentName)
			altName = fmt.Sprintf("continent=%s", aws.ToString(details.ContinentCode))
		}
		if name == "" {
			// should never happen
			continue
		}
		location := &route53types.GeoLocation{
			ContinentCode:   details.ContinentCode,
			CountryCode:     details.CountryCode,
			SubdivisionCode: details.SubdivisionCode,
		}
		r.cachedGeoLocationNameToLocation[name] = location
		r.cachedGeoLocationNameToLocation[altName] = location
		code := codeFromGeoLocation(location)
		r.cachedGeoLocationCodeToName[code] = name
	}
	return nil
}

func (r *routingPolicyContext) lookupCIDRRoutingConfig(ctx context.Context, collectionName, locationName string) (*route53types.CidrRoutingConfig, error) {
	r.Lock()
	defer r.Unlock()

	// refresh every hour
	if err := r.refreshCIDRCollections(ctx, refreshCIDRCollectionsPeriod, "", collectionName); err != nil {
		return nil, err
	}

	collectionID := r.cachedCIDRCollectionNameToID[collectionName]
	// if not found try after refresh, but only if last lookup older than 5 minutes
	if collectionID == "" {
		if err := r.refreshCIDRCollections(ctx, refreshCIDRCollectionsPeriodNotFound, "", collectionName); err != nil {
			return nil, err
		}
		collectionID = r.cachedCIDRCollectionNameToID[collectionName]
	}

	if collectionID == "" {
		return nil, fmt.Errorf("CIDR collection named %s not found", collectionName)
	}

	blocks := r.cachedCIDRCollectionIDToBlocks[collectionID]
	if blocks.locationToCIDRBlock == nil {
		return nil, fmt.Errorf("CIDR collection ID %s not found", collectionID)
	}
	if locationName != "*" && blocks.locationToCIDRBlock[locationName] == "" {
		if err := r.refreshCIDRCollections(ctx, refreshCIDRCollectionsPeriodNotFound, collectionID, collectionName); err != nil {
			return nil, err
		}
	}
	if locationName != "*" && blocks.locationToCIDRBlock[locationName] == "" {
		return nil, fmt.Errorf("location name %s not found in CIDR collection named %s", locationName, collectionName)
	}
	return &route53types.CidrRoutingConfig{
		CollectionId: aws.String(collectionID),
		LocationName: aws.String(locationName),
	}, nil
}

func (r *routingPolicyContext) collectionName(ctx context.Context, collectionID string) (string, error) {
	r.Lock()
	defer r.Unlock()

	// refresh every 24 hours
	if err := r.refreshCIDRCollections(ctx, refreshCIDRCollectionsPeriod, collectionID, ""); err != nil {
		return "", err
	}

	collectionName := r.cachedCIDRCollectionIDToName[collectionID]
	// if not found try after refresh, but only if last lookup older than 10 minutes
	if collectionName == "" {
		if err := r.refreshCIDRCollections(ctx, refreshCIDRCollectionsPeriodNotFound, collectionID, ""); err != nil {
			return "", err
		}
		collectionName = r.cachedCIDRCollectionIDToName[collectionID]
	}

	if collectionName == "" {
		return "", fmt.Errorf("CIDR collection ID %s not found", collectionID)
	}

	return collectionName, nil
}

func (r *routingPolicyContext) refreshCIDRCollections(ctx context.Context, period time.Duration, collectionID, collectionName string) error {
	if collectionID == "" && collectionName == "" {
		return fmt.Errorf("missing collection ID or name")
	}

	if r.lastCIDRCollectionUpdate.Add(period).Before(time.Now()) {
		output, err := r.r53.ListCidrCollections(ctx, &route53.ListCidrCollectionsInput{})
		if err != nil {
			r.lastCIDRCollectionUpdate = time.Now().Add(-period / 2)
			return fmt.Errorf("listing CIDR collections failed: %s", err)
		}

		r.lastCIDRCollectionUpdate = time.Now()
		r.cachedCIDRCollectionIDToName = map[string]string{}
		r.cachedCIDRCollectionNameToID = map[string]string{}
		for _, item := range output.CidrCollections {
			id := aws.ToString(item.Id)
			name := aws.ToString(item.Name)
			r.cachedCIDRCollectionIDToName[id] = name
			r.cachedCIDRCollectionNameToID[name] = id
		}

		for id := range r.cachedCIDRCollectionIDToBlocks {
			if _, ok := r.cachedCIDRCollectionIDToName[id]; !ok {
				delete(r.cachedCIDRCollectionIDToBlocks, id)
				delete(r.lastCIDRCollectionBlocksUpdate, id)
			}
		}
	}

	if collectionID != "" {
		collectionName = r.cachedCIDRCollectionIDToName[collectionID]
		if collectionName == "" {
			return fmt.Errorf("unknown CIDR collection ID: %s", collectionID)
		}
	} else {
		collectionID = r.cachedCIDRCollectionNameToID[collectionName]
		if collectionID == "" {
			return fmt.Errorf("unknown CIDR collection name: %s", collectionName)
		}
	}

	lastUpdate := r.lastCIDRCollectionBlocksUpdate[collectionID]
	if lastUpdate.Add(period).Before(time.Now()) {
		output, err := r.r53.ListCidrBlocks(ctx, &route53.ListCidrBlocksInput{CollectionId: aws.String(collectionID)})
		if err != nil {
			r.lastCIDRCollectionBlocksUpdate[collectionID] = time.Now().Add(-period / 2)
			return fmt.Errorf("listing CIDR blocks failed for %s (%s)", collectionID, collectionName)
		}

		r.lastCIDRCollectionBlocksUpdate[collectionID] = time.Now()
		blocks := cidrBlockMap{
			locationToCIDRBlock: map[string]string{},
			cidrBlockToLocation: map[string]string{},
		}
		for _, item := range output.CidrBlocks {
			locationName := aws.ToString(item.LocationName)
			cidrBlock := aws.ToString(item.CidrBlock)
			blocks.locationToCIDRBlock[locationName] = cidrBlock
			blocks.cidrBlockToLocation[cidrBlock] = locationName
		}
		r.cachedCIDRCollectionIDToBlocks[collectionID] = blocks
	}

	return nil
}

func (r *routingPolicyContext) addRoutingPolicy(ctx context.Context, rrset *route53types.ResourceRecordSet, name dns.DNSSetName, routingPolicy *dns.RoutingPolicy) error {
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
	var optionalKeys []string
	switch routingPolicy.Type {
	case dns.RoutingPolicyWeighted:
		keys = []string{keyWeight}
		optionalKeys = []string{keyHealthCheckID}
	case dns.RoutingPolicyLatency:
		keys = []string{keyRegion}
		optionalKeys = []string{keyHealthCheckID}
	case dns.RoutingPolicyGeoLocation:
		keys = []string{keyLocation}
		optionalKeys = []string{keyHealthCheckID}
	case dns.RoutingPolicyIPBased:
		keys = []string{keyCollection, keyLocation}
		optionalKeys = []string{keyHealthCheckID}
	case dns.RoutingPolicyFailover:
		keys = []string{keyFailoverRecordType}
		optionalKeys = []string{keyDisableEvaluateTargetHealth, keyHealthCheckID}
	default:
		return fmt.Errorf("unsupported routing policy type %s", routingPolicy.Type)
	}

	if err := routingPolicy.CheckParameterKeys(keys, optionalKeys); err != nil {
		return err
	}

	rrset.SetIdentifier = aws.String(name.SetIdentifier)
	for key, value := range routingPolicy.Parameters {
		switch key {
		case keyWeight:
			v, err := strconv.ParseInt(value, 0, 64)
			if err != nil || v < 0 {
				return fmt.Errorf("invalid value for spec.routingPolicy.parameters.weight: %s", value)
			}
			rrset.Weight = aws.Int64(v)
		case keyRegion:
			rrset.Region = route53types.ResourceRecordSetRegion(value)
		case keyLocation:
			switch routingPolicy.Type {
			case dns.RoutingPolicyGeoLocation:
				geoLocation, err := r.lookupGeoLocation(ctx, value)
				if err != nil {
					return err
				}
				rrset.GeoLocation = geoLocation
			case dns.RoutingPolicyIPBased:
				collection := routingPolicy.Parameters[keyCollection]
				cidrRoutingConfig, err := r.lookupCIDRRoutingConfig(ctx, collection, value)
				if err != nil {
					return err
				}
				rrset.CidrRoutingConfig = cidrRoutingConfig
			}
		case keyFailoverRecordType:
			upperValue := strings.ToUpper(value)
			valid := false
			for _, validValue := range route53types.ResourceRecordSetFailoverPrimary.Values() {
				if string(validValue) == upperValue {
					valid = true
					rrset.Failover = validValue
					break
				}
			}
			if !valid {
				return fmt.Errorf("invalid %s value: %s", keyFailoverRecordType, value)
			}
		case keyDisableEvaluateTargetHealth:
			if rrset.AliasTarget == nil {
				return fmt.Errorf("%s only allowed for alias targets", keyDisableEvaluateTargetHealth)
			}
			disabled, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid %s value: %s", keyDisableEvaluateTargetHealth, value)
			}
			if disabled {
				rrset.AliasTarget.EvaluateTargetHealth = false
			}
		case keyHealthCheckID:
			rrset.HealthCheckId = aws.String(value)
		}
	}

	return nil
}

func (r *routingPolicyContext) extractRoutingPolicy(ctx context.Context, rrset *route53types.ResourceRecordSet) *dns.RoutingPolicy {
	if rrset.SetIdentifier == nil {
		return nil
	}

	var keyvalues []string
	if rrset.HealthCheckId != nil {
		keyvalues = []string{keyHealthCheckID, aws.ToString(rrset.HealthCheckId)}
	}

	if rrset.Weight != nil {
		keyvalues = append(keyvalues, keyWeight, strconv.FormatInt(*rrset.Weight, 10))
		return dns.NewRoutingPolicy(dns.RoutingPolicyWeighted, keyvalues...)
	}

	if rrset.Region != "" {
		keyvalues = append(keyvalues, keyRegion, string(rrset.Region))
		return dns.NewRoutingPolicy(dns.RoutingPolicyLatency, keyvalues...)
	}

	if rrset.GeoLocation != nil {
		location, err := r.geoLocationName(ctx, rrset.GeoLocation)
		if err != nil {
			// ignore
			return nil
		}
		keyvalues = append(keyvalues, keyLocation, location)
		return dns.NewRoutingPolicy(dns.RoutingPolicyGeoLocation, keyvalues...)
	}

	if rrset.CidrRoutingConfig != nil {
		collectioName, err := r.collectionName(ctx, aws.ToString(rrset.CidrRoutingConfig.CollectionId))
		if err != nil {
			// ignore
			return nil
		}
		keyvalues = append(keyvalues, keyCollection, collectioName, keyLocation, aws.ToString(rrset.CidrRoutingConfig.LocationName))
		return dns.NewRoutingPolicy(dns.RoutingPolicyGeoLocation, keyvalues...)
	}

	if rrset.Failover != "" {
		if rrset.AliasTarget != nil && rrset.AliasTarget.EvaluateTargetHealth {
			// only store false value, as true is default
			keyvalues = append(keyvalues, keyDisableEvaluateTargetHealth, "true")
		}
		keyvalues = append(keyvalues, keyFailoverRecordType, string(rrset.Failover))
		return dns.NewRoutingPolicy(dns.RoutingPolicyFailover, keyvalues...)
	}
	// ignore unsupported routing policy
	return nil
}

func codeFromGeoLocation(location *route53types.GeoLocation) string {
	if location == nil {
		return ""
	}
	return fmt.Sprintf("%s,%s,%s", aws.ToString(location.ContinentCode), aws.ToString(location.CountryCode), aws.ToString(location.SubdivisionCode))
}
