// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// DefaultDNSHostedZone is the default implementation for DNSHostedZone.
type DefaultDNSHostedZone struct {
	zoneid    dns.ZoneID // qualified zone id
	domain    string     // base domain for zone
	key       string     // internal key used by provider (not used by this lib)
	isPrivate bool       // indicates a private zone
}

var _ DNSHostedZone = &DefaultDNSHostedZone{}

func (z *DefaultDNSHostedZone) Key() string {
	if z.key != "" {
		return z.key
	}
	return z.zoneid.ID
}

func (z *DefaultDNSHostedZone) ZoneID() dns.ZoneID {
	return z.zoneid
}

func (z *DefaultDNSHostedZone) Domain() string {
	return z.domain
}

func (z *DefaultDNSHostedZone) IsPrivate() bool {
	return z.isPrivate
}

func (z *DefaultDNSHostedZone) MatchLevel(dnsname string) int {
	return MatchLevel(z, dnsname)
}

// MatchLevel returns the match level of the given DNS name with the hosted zone.
func MatchLevel(zone DNSHostedZone, dnsname string) int {
	if dnsutils.Match(dnsname, zone.Domain()) {
		return len(zone.Domain())
	}
	return 0
}

// NewDNSHostedZone creates a new DNS hosted zone.
func NewDNSHostedZone(providerType, id, domain, key string, isPrivate bool) DNSHostedZone {
	return &DefaultDNSHostedZone{zoneid: dns.NewZoneID(providerType, id), key: key, domain: domain, isPrivate: isPrivate}
}
