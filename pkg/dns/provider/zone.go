// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/utils"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type dnsHostedZones map[dns.ZoneID]*dnsHostedZone

type dnsHostedZone struct {
	*dnsutils.RateLimiter
	lock        sync.Mutex
	busy        bool
	zone        DNSHostedZone
	next        time.Time
	nextTrigger time.Duration
	policy      *dnsHostedZonePolicy
}

func newDNSHostedZone(min time.Duration, zone DNSHostedZone) *dnsHostedZone {
	return &dnsHostedZone{
		zone:        zone,
		RateLimiter: dnsutils.NewRateLimiter(min, 10*time.Minute),
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

func (this *dnsHostedZone) Id() dns.ZoneID {
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
