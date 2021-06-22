/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package provider

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	utils2 "github.com/gardener/controller-manager-library/pkg/utils"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

////////////////////////////////////////////////////////////////////////////////
// state handling for DNSHostedZonePolicies
////////////////////////////////////////////////////////////////////////////////

func (this *state) UpdateZonePolicy(logger logger.LogContext, policy *dnsutils.DNSHostedZonePolicyObject) reconcile.Status {
	zones, conflicts := this.updateZonePolicyState(logger, policy)

	err := this.updateZonePolicyStatus(policy, zones, conflicts)
	if err != nil {
		reconcile.Delay(logger, err)
	}

	return reconcile.Succeeded(logger)
}

func (this *state) updateZonePolicyState(logger logger.LogContext, policy *dnsutils.DNSHostedZonePolicyObject) ([]api.ZoneInfo, []string) {
	this.lock.Lock()
	defer this.lock.Unlock()

	name := policy.GetName()
	pol := this.zonePolicies[name]
	if pol == nil {
		pol = newDNSHostedZonePolicy(name, policy.Spec())
		this.zonePolicies[name] = pol
	} else {
		pol.spec = *policy.Spec()
	}

	var conflicts []string
	var zones []api.ZoneInfo
	pol.zones = nil
	pol.conflictingPolicyNames.Clear()
	for _, zone := range this.zones {
		if matchesPolicySelector(pol, zone) {
			if zpol := zone.Policy(); zpol == nil {
				zone.SetPolicy(pol)
				logger.Infof("added zone %s to policy %s", zone.Id(), name)
			} else if zpol != pol {
				zname := zpol.name
				s := fmt.Sprintf("zone %s has conflicting policy selection: %s", zone.Id(), zname)
				conflicts = append(conflicts, s)
				pol.conflictingPolicyNames.Add(zname)
				zpol.conflictingPolicyNames.Add(name)
			}
		} else if zone.Policy() == pol {
			zone.SetPolicy(nil)
			logger.Infof("removed zone %s to policy %s", zone.Id(), name)
		}
		if zone.Policy() == pol {
			pol.zones = append(pol.zones, zone)
			zones = append(zones, api.ZoneInfo{
				ZoneID:       zone.Id(),
				ProviderType: zone.ProviderType(),
				DomainName:   zone.Domain(),
			})
		}
	}
	this.updateStateTTLMap()
	return zones, conflicts
}

func (this *state) updateStateTTLMap() {
	new := map[string]time.Duration{}
	for _, zone := range this.zones {
		if zpol := zone.Policy(); zpol != nil {
			if zpol.spec.Policy.ZoneStateCacheTTL != nil {
				new[zone.Id()] = zpol.spec.Policy.ZoneStateCacheTTL.Duration
			}
		}
	}
	this.zoneStateTTL.Store(new)
}

func (this *state) RemoveZonePolicy(logger logger.LogContext, policy *dnsutils.DNSHostedZonePolicyObject) reconcile.Status {
	key := this.createZonePolicyClusterKey(policy.GetName())
	return this.ZonePolicyDeleted(logger, key)
}

func (this *state) ZonePolicyDeleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	this.lock.Lock()
	defer this.lock.Unlock()

	name := key.Name()
	if pol := this.zonePolicies[name]; pol != nil {
		for _, zone := range pol.zones {
			zone.SetPolicy(nil)
		}
		for zname := range pol.conflictingPolicyNames {
			key := this.createZonePolicyClusterKey(zname)
			this.triggerKey(key)
		}
		this.updateStateTTLMap()
		delete(this.zonePolicies, name)
	}

	return reconcile.Succeeded(logger)
}

func (this *state) updateZonePolicyStatus(policy *dnsutils.DNSHostedZonePolicyObject,
	zones []api.ZoneInfo, conflicts []string) error {

	var pmsg *string
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		msg := strings.Join(conflicts, ", ")
		pmsg = &msg
	}

	sort.Slice(zones, func(i, j int) bool {
		return zones[i].DomainName < zones[j].DomainName ||
			zones[i].DomainName == zones[j].DomainName && zones[i].ZoneID < zones[j].ZoneID
	})

	status := policy.Status()
	mod := &utils2.ModificationState{}
	mod.AssureStringPtrPtr(&status.Message, pmsg)
	if !reflect.DeepEqual(status.Zones, zones) {
		status.Zones = zones
		n := len(zones)
		status.Count = &n
		mod.Modify(true)
	}

	if mod.IsModified() {
		status.LastStatusUpdateTime = &metav1.Time{Time: time.Now()}
		return policy.UpdateStatus()
	}

	return nil
}

func (this *state) triggerAllZonePolicies() {
	for name := range this.zonePolicies {
		key := this.createZonePolicyClusterKey(name)
		this.triggerKey(key)
	}
}

func (this *state) createZonePolicyClusterKey(name string) resources.ClusterObjectKey {
	providerClusterID := this.context.GetCluster(PROVIDER_CLUSTER).GetId()
	return resources.NewClusterKey(providerClusterID, zonePolicyGroupKind, "", name)
}

func matchesPolicySelector(pol *dnsHostedZonePolicy, zone *dnsHostedZone) bool {
	selector := &pol.spec.Selector
	found := findFullMatch(selector.ZoneIDs, zone.Id())
	found = found && findFullMatch(selector.ProviderTypes, zone.ProviderType())
	found = found && findFullMatch(selector.DomainNames, zone.Domain())
	return found
}

func findFullMatch(list []string, key string) bool {
	if len(list) == 0 {
		return true
	}
	for _, item := range list {
		if item == key {
			return true
		}
	}
	return false
}
