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

import "github.com/gardener/external-dns-management/pkg/dns"


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

////////////////////////////////////////////////////////////////////////////////
//  Default Implementation for DNSHostedZone
////////////////////////////////////////////////////////////////////////////////

type DefaultDNSHostedZone struct {
	id        string   // identifying id for provider api
	domain    string   // base domain for zone
	forwarded []string // forwarded sub domains
	key       string   // internal key used by provider (not used by this lib)
}

func (this *DefaultDNSHostedZone) Key() string {
	if this.key != "" {
		return this.key
	}
	return this.id
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

func NewDNSHostedZone(id, domain, key string, forwarded []string) DNSHostedZone {
	return &DefaultDNSHostedZone{id:id, key:key, domain:domain, forwarded: forwarded}
}