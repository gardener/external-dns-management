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
)

type dnsHostedZones map[string]*dnsHostedZone

type dnsHostedZone struct {
	lock sync.Mutex
	busy bool
	zone DNSHostedZone
	next time.Time
}

func newDNSHostedZone(zone DNSHostedZone) *dnsHostedZone {
	return &dnsHostedZone{
		zone: zone,
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
	return fmt.Sprintf("%s: %s", this.zone.Id(), this.zone.Domain())
}

func (this *dnsHostedZone) Release() {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.busy = false
}

func (this *dnsHostedZone) ProviderType() string {
	return this.zone.ProviderType()
}

func (this *dnsHostedZone) Id() string {
	return this.zone.Id()
}

func (this *dnsHostedZone) Domain() string {
	return this.zone.Domain()
}

func (this *dnsHostedZone) ForwardedDomains() []string {
	return this.zone.ForwardedDomains()
}

////////////////////////////////////////////////////////////////////////////////

func (this *dnsHostedZone) update(zone DNSHostedZone) {
	this.zone = zone
}
