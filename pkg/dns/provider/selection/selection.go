// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package selection

import (
	"fmt"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
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
	Id() dns.ZoneID
	Domain() string
	ForwardedDomains() []string
}

// CalcZoneAndDomainSelection calculates the effective included/excluded domains and zones for the given spec and
// zones supported by a provider.
func CalcZoneAndDomainSelection(spec v1alpha1.DNSProviderSpec, allzones []LightDNSHostedZone) SelectionResult {
	this := SelectionResult{
		SpecDomainSel: PrepareSelection(spec.Domains),
		SpecZoneSel:   PrepareSelection(spec.Zones),
		ZoneSel:       NewSubSelection(),
		DomainSel:     NewSubSelection(),
	}

	if err := validateDomains(this.SpecDomainSel.Include, "domains include"); err != nil {
		this.Error = err.Error()
		return this
	}
	if err := validateDomains(this.SpecDomainSel.Exclude, "domains exclude"); err != nil {
		this.Error = err.Error()
		return this
	}

	var zones []LightDNSHostedZone
	for _, z := range allzones {
		if z.Id().ProviderType == spec.Type {
			zones = append(zones, z)
		}
	}

	if len(this.SpecZoneSel.Include) > 0 {
		for _, z := range zones {
			if this.SpecZoneSel.Include.Contains(z.Id().ID) {
				this.ZoneSel.Include.Add(z.Id().ID)
			} else {
				this.ZoneSel.Exclude.Add(z.Id().ID)
			}
		}
	} else {
		for _, z := range zones {
			this.ZoneSel.Include.Add(z.Id().ID)
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
		if this.ZoneSel.Include.Contains(z.Id().ID) {
			this.Zones = append(this.Zones, z)
		}
	}

	if len(zones) > 0 && len(this.Zones) == 0 {
		this.Error = "no zone available in account matches zone filter"
		return this
	}

	var err error
	this.DomainSel.Include, err = filterByZones(normalizeDomains(this.SpecDomainSel.Include), this.Zones)
	if err != nil {
		this.Warnings = append(this.Warnings, err.Error())
	}
	this.DomainSel.Exclude, err = filterByZones(normalizeDomains(this.SpecDomainSel.Exclude), this.Zones)
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
			zoneDomains := []string{}
			for _, z := range this.Zones {
				zoneDomains = append(zoneDomains, z.Domain())
			}
			this.Zones = nil
			this.Error = fmt.Sprintf("no domain matching hosting zones. Need to be a (sub)domain of [%s]",
				strings.Join(zoneDomains, ", "))
			for _, z := range allzones {
				this.DomainSel.Exclude.Add(z.Domain())
			}
			return this
		}
	}

	excludedSubdomains := excludeForwardedSubdomains(this.DomainSel.Include, this.Zones)
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
		this.ZoneSel.Include.Remove(zone.Id().ID)
		this.ZoneSel.Exclude.Add(zone.Id().ID)
	}

	for _, z := range allzones {
		if !this.ZoneSel.Include.Contains(z.Id().ID) && !this.DomainSel.Include.Contains(z.Domain()) {
			this.DomainSel.Exclude.Add(z.Domain())
		}
	}
	if len(this.ZoneSel.Include) != len(this.Zones) {
		this.Zones = nil
		for _, z := range zones {
			if this.ZoneSel.Include.Contains(z.Id().ID) {
				this.Zones = append(this.Zones, z)
			}
		}
	}

	return this
}

func validateDomains(domains utils.StringSet, name string) error {
	for domain := range domains {
		if strings.HasPrefix(domain, "*.") {
			return fmt.Errorf("wildcards are not allowed in %s '%s' (hint: remove the wildcard)", name, domain)
		}
	}
	return nil
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

// excludeForwardedSubdomains excludes all forwarded subdomains
func excludeForwardedSubdomains(includedDomains utils.StringSet, zones []LightDNSHostedZone) utils.StringSet {
	exclude := utils.StringSet{}
	for d := range includedDomains {
		for _, z := range zones {
			if dnsutils.Match(d, z.Domain()) {
				for _, sub := range z.ForwardedDomains() {
					if dnsutils.Match(sub, d) && !includedDomains.Contains(sub) {
						exclude.Add(sub)
					}
				}
			}
		}
	}
	return exclude
}

func normalizeDomains(domains utils.StringSet) utils.StringSet {
	if len(domains) == 0 {
		return domains
	}

	normalized := utils.NewStringSet()
	for k := range domains {
		k = strings.TrimSuffix(strings.ToLower(k), ".")
		normalized.Add(k)
	}
	return normalized
}
