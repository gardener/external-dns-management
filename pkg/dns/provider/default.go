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
	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

////////////////////////////////////////////////////////////////////////////////
//  Default Implementation for DNSZoneState
////////////////////////////////////////////////////////////////////////////////

type DefaultDNSZoneState struct {
	sets dns.DNSSets
}

func (this *DefaultDNSZoneState) GetDNSSets() dns.DNSSets {
	return this.sets
}

func NewDNSZoneState(sets dns.DNSSets) DNSZoneState {
	return &DefaultDNSZoneState{sets}
}

func (this *DefaultDNSZoneState) Clone() DNSZoneState {
	return NewDNSZoneState(this.sets.Clone())
}

////////////////////////////////////////////////////////////////////////////////
//  Default Implementation for DNSHostedZone
////////////////////////////////////////////////////////////////////////////////

type DefaultDNSHostedZone struct {
	providerType string   // provider type
	id           string   // identifying id for provider api
	domain       string   // base domain for zone
	forwarded    []string // forwarded sub domains
	key          string   // internal key used by provider (not used by this lib)
	isPrivate    bool     // indicates a private zone
}

func (this *DefaultDNSHostedZone) Key() string {
	if this.key != "" {
		return this.key
	}
	return this.id
}

func (this *DefaultDNSHostedZone) ProviderType() string {
	return this.providerType
}

func (this *DefaultDNSHostedZone) Id() string {
	return this.id
}

func (this *DefaultDNSHostedZone) Domain() string {
	return this.domain
}

func (this *DefaultDNSHostedZone) ForwardedDomains() []string {
	return this.forwarded
}

func (this *DefaultDNSHostedZone) IsPrivate() bool {
	return this.isPrivate
}

func (this *DefaultDNSHostedZone) Match(dnsname string) int {
	return Match(this, dnsname)
}

func Match(zone DNSHostedZone, dnsname string) int {
	for _, forwardedDomain := range zone.ForwardedDomains() {
		if dnsutils.Match(dnsname, forwardedDomain) {
			return 0
		}
	}
	if dnsutils.Match(dnsname, zone.Domain()) {
		return len(zone.Domain())
	}
	return 0
}

func NewDNSHostedZone(ptype string, id, domain, key string, forwarded []string, isPrivate bool) DNSHostedZone {
	return &DefaultDNSHostedZone{providerType: ptype, id: id, key: key, domain: domain, forwarded: forwarded, isPrivate: isPrivate}
}

func CopyDNSHostedZone(zone DNSHostedZone, forwardedDomains []string) DNSHostedZone {
	return &DefaultDNSHostedZone{providerType: zone.ProviderType(), id: zone.Id(), key: zone.Key(),
		domain: zone.Domain(), forwarded: forwardedDomains, isPrivate: zone.IsPrivate()}
}
