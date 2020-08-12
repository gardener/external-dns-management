/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package selection

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/utils"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

// SelectionResult contains the result of the CalcZoneAndDomainSelection function
type SelectionResult struct {
	Zones         []LightDNSHostedZone
	SpecZoneSel   SubSelection
	SpecDomainSel SubSelection
	ZoneSel       SubSelection
	DomainSel     SubSelection
	Error         string
	Warnings      []string
}

// SubSelection contains an included and an excluded string set
type SubSelection struct {
	Include utils.StringSet
	Exclude utils.StringSet
}

// NewSubSelection creates an empty SubSelection
func NewSubSelection() SubSelection {
	return SubSelection{
		Include: utils.NewStringSet(),
		Exclude: utils.NewStringSet(),
	}
}

// LightDNSHostedZone contains the info of a DNSHostedZone needed for selection
type LightDNSHostedZone interface {
	Id() string
	Domain() string
	ForwardedDomains() []string
}

// CalcZoneAndDomainSelection calculates the effective included/excluded domains and zones for the given spec and
// zones supported by a provider.
func CalcZoneAndDomainSelection(spec v1alpha1.DNSProviderSpec, zones []LightDNSHostedZone) SelectionResult {
	this := SelectionResult{
		SpecDomainSel: PrepareSelection(spec.Domains),
		SpecZoneSel:   PrepareSelection(spec.Zones),
		ZoneSel:       NewSubSelection(),
		DomainSel:     NewSubSelection(),
	}

	if len(this.SpecZoneSel.Include) > 0 {
		for _, z := range zones {
			if this.SpecZoneSel.Include.Contains(z.Id()) {
				this.ZoneSel.Include.Add(z.Id())
			} else {
				this.ZoneSel.Exclude.Add(z.Id())
			}
		}
	} else {
		for _, z := range zones {
			this.ZoneSel.Include.Add(z.Id())
		}
	}
	if len(this.SpecZoneSel.Exclude) > 0 {
		for id := range this.ZoneSel.Include {
			if this.SpecZoneSel.Exclude.Contains(id) {
				this.ZoneSel.Include.Remove(id)
				this.ZoneSel.Exclude.Add(id)
			}
		}
	}
	for _, z := range zones {
		if this.ZoneSel.Include.Contains(z.Id()) {
			this.Zones = append(this.Zones, z)
		}
	}

	if len(zones) > 0 && len(this.Zones) == 0 {
		this.Error = "no zone available in account matches zone filter"
		return this
	}

	var err error
	this.DomainSel.Include, err = filterByZones(this.SpecDomainSel.Include, this.Zones)
	if err != nil {
		this.Warnings = append(this.Warnings, err.Error())
	}
	this.DomainSel.Exclude, err = filterByZones(this.SpecDomainSel.Exclude, this.Zones)
	if err != nil {
		this.Warnings = append(this.Warnings, err.Error())
	}

	if len(this.SpecDomainSel.Include) == 0 {
		if len(this.Zones) == 0 {
			this.Error = "no hosted zones found"
			return this
		}
		for _, z := range this.Zones {
			this.DomainSel.Include.Add(z.Domain())
		}
	} else {
		if len(this.DomainSel.Include) == 0 {
			this.ZoneSel.Exclude.AddSet(this.ZoneSel.Include)
			this.ZoneSel.Include = utils.NewStringSet()
			this.Zones = nil
			this.Error = "no domain matching hosting zones"
			return this
		}
	}

	includedSubdomains, excludedSubdomains := collectForwardedSubdomains(this.DomainSel.Include, this.DomainSel.Exclude, this.Zones)
	this.DomainSel.Include.AddSet(includedSubdomains)
	this.DomainSel.Exclude.AddSet(excludedSubdomains)

outer:
	for _, zone := range this.Zones {
		for domain := range this.DomainSel.Include {
			if dnsutils.Match(domain, zone.Domain()) {
				ok := true
				for _, fd := range zone.ForwardedDomains() {
					if dnsutils.Match(domain, fd) {
						ok = false
						break
					}
				}
				if ok {
					continue outer
				}
			}
		}
		this.ZoneSel.Include.Remove(zone.Id())
		this.ZoneSel.Exclude.Add(zone.Id())
	}

	if len(this.ZoneSel.Include) != len(this.Zones) {
		this.Zones = nil
		for _, z := range zones {
			if this.ZoneSel.Include.Contains(z.Id()) {
				this.Zones = append(this.Zones, z)
			}
		}
	}

	return this
}

func PrepareSelection(sel *v1alpha1.DNSSelection) SubSelection {
	subSel := NewSubSelection()
	if sel != nil {
		subSel.Include = utils.NewStringSetByArray(sel.Include)
		subSel.Exclude = utils.NewStringSetByArray(sel.Exclude)
	}
	return subSel
}

func filterByZones(domains utils.StringSet, zones []LightDNSHostedZone) (result utils.StringSet, err error) {
	result = utils.StringSet{}
	for d := range domains {
	_zones:
		for _, z := range zones {
			if dnsutils.Match(d, z.Domain()) {
				for _, sub := range z.ForwardedDomains() {
					if dnsutils.Match(d, sub) {
						continue _zones
					}
				}
				result.Add(d)
				break
			}
		}
		if !result.Contains(d) {
			err = fmt.Errorf("domain %q not in hosted domains", d)
		}
	}
	return result, err
}

// collectForwardedSubdomains excluded all forwarded subdomains
// but keeps accessible sub hosted zones
func collectForwardedSubdomains(includedDomains utils.StringSet, excludedDomains utils.StringSet,
	zones []LightDNSHostedZone) (utils.StringSet, utils.StringSet) {
	include := utils.StringSet{}
	exclude := utils.StringSet{}
	for d := range includedDomains {
		for _, z := range zones {
			if d == z.Domain() {
				for _, sub := range z.ForwardedDomains() {
					if !includedDomains.Contains(sub) {
						exclude.Add(sub)
					}
				}
			}
		}
	}
	for _, z := range zones {
		if !excludedDomains.Contains(z.Domain()) && exclude.Contains(z.Domain()) {
			exclude.Remove(z.Domain())
			include.Add(z.Domain())
		}
	}
	return include, exclude
}
