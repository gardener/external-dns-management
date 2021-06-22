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
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/utils"
	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type dnsHostedZones map[string]*dnsHostedZone

type dnsHostedZone struct {
	*dnsutils.RateLimiter
	lock   sync.Mutex
	busy   bool
	zone   DNSHostedZone
	next   time.Time
	owners utils.StringSet
	policy *dnsHostedZonePolicy
}

func newDNSHostedZone(min time.Duration, zone DNSHostedZone) *dnsHostedZone {
	return &dnsHostedZone{
		zone:        zone,
		RateLimiter: dnsutils.NewRateLimiter(min, 10*time.Minute, min/2),
		owners:      utils.StringSet{},
	}
}

func (this *dnsHostedZone) TestAndSetBusy() bool {
	this.lock.Lock()
	defer this.lock.Unlock()

	if this.busy {
		return false
	}
	this.busy = true
	return true
}

func (this *dnsHostedZone) String() string {
	zone := this.getZone()
	return fmt.Sprintf("%s: %s", zone.Id(), zone.Domain())
}

func (this *dnsHostedZone) Release() {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.busy = false
}

func (this *dnsHostedZone) getZone() DNSHostedZone {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.zone
}

func (this *dnsHostedZone) ProviderType() string {
	return this.getZone().ProviderType()
}

func (this *dnsHostedZone) Id() string {
	return this.getZone().Id()
}

func (this *dnsHostedZone) Domain() string {
	return this.getZone().Domain()
}

func (this *dnsHostedZone) ForwardedDomains() []string {
	return this.getZone().ForwardedDomains()
}

func (this *dnsHostedZone) Key() string {
	return this.getZone().Key()
}

func (this *dnsHostedZone) IsPrivate() bool {
	return this.getZone().IsPrivate()
}

func (this *dnsHostedZone) Match(dnsname string) int {
	return Match(this, dnsname)
}

func (this *dnsHostedZone) SetOwners(owners utils.StringSet) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.owners = owners
}

func (this *dnsHostedZone) IntersectOwners(owners utils.StringSet) utils.StringSet {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.owners.Intersect(owners)
}

func (this *dnsHostedZone) GetNext() time.Time {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.next
}

func (this *dnsHostedZone) SetNext(next time.Time) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.next = next
}

func (this *dnsHostedZone) Policy() *dnsHostedZonePolicy {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.policy
}

func (this *dnsHostedZone) SetPolicy(pol *dnsHostedZonePolicy) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.policy = pol
}

////////////////////////////////////////////////////////////////////////////////

func (this *dnsHostedZone) update(zone DNSHostedZone) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.zone = zone
}

////////////////////////////////////////////////////////////////////////////////

type dnsHostedZonePolicy struct {
	name                   string
	spec                   dnsv1alpha1.DNSHostedZonePolicySpec
	zones                  []*dnsHostedZone
	conflictingPolicyNames utils.StringSet
}

func newDNSHostedZonePolicy(name string, spec *dnsv1alpha1.DNSHostedZonePolicySpec) *dnsHostedZonePolicy {
	return &dnsHostedZonePolicy{
		name:                   name,
		spec:                   *spec,
		conflictingPolicyNames: utils.StringSet{},
	}
}
