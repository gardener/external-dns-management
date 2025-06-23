// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
	zoneid    dns.ZoneID // qualified zone id
	domain    string     // base domain for zone
	forwarded []string   // forwarded sub domains
	key       string     // internal key used by provider (not used by this lib)
	isPrivate bool       // indicates a private zone
}

func (this *DefaultDNSHostedZone) Key() string {
	if this.key != "" {
		return this.key
	}
	return this.zoneid.ID
}

func (this *DefaultDNSHostedZone) Id() dns.ZoneID {
	return this.zoneid
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

func NewDNSHostedZone(providerType, id, domain, key string, isPrivate bool) DNSHostedZone {
	return &DefaultDNSHostedZone{zoneid: dns.NewZoneID(providerType, id), key: key, domain: domain, isPrivate: isPrivate}
}

func CopyDNSHostedZone(zone DNSHostedZone, forwardedDomains []string) DNSHostedZone {
	return &DefaultDNSHostedZone{
		zoneid: zone.Id(), key: zone.Key(),
		domain: zone.Domain(), forwarded: forwardedDomains, isPrivate: zone.IsPrivate(),
	}
}
