// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package selection

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
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
	Include sets.Set[string]
	Exclude sets.Set[string]
}

// NewSubSelection creates an empty SubSelection
func NewSubSelection() SubSelection {
	return SubSelection{
		Include: sets.New[string](),
		Exclude: sets.New[string](),
	}
}

// LightDNSHostedZone contains the info of a DNSHostedZone needed for selection
type LightDNSHostedZone interface {
	ZoneID() dns.ZoneID
	Domain() string
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

	forwardedZones := map[string][]LightDNSHostedZone{}
	for _, z1 := range allzones {
		for _, z2 := range allzones {
			if z1 == z2 {
				continue
			}
			if dnsutils.Match(z1.Domain(), z2.Domain()) && z1.Domain() != z2.Domain() {
				forwardedZones[z2.ZoneID().ID] = append(forwardedZones[z2.ZoneID().ID], z1)
			}
		}
	}

	var zones []LightDNSHostedZone
	for _, z := range allzones {
		if z.ZoneID().ProviderType == spec.Type {
			zones = append(zones, z)
		}
	}

	if len(this.SpecZoneSel.Include) > 0 {
		for _, z := range zones {
			if this.SpecZoneSel.Include.Has(z.ZoneID().ID) {
				this.ZoneSel.Include.Insert(z.ZoneID().ID)
			} else {
				this.ZoneSel.Exclude.Insert(z.ZoneID().ID)
			}
		}
	} else {
		for _, z := range zones {
			this.ZoneSel.Include.Insert(z.ZoneID().ID)
		}
	}
	if len(this.SpecZoneSel.Exclude) > 0 {
		for id := range this.ZoneSel.Include {
			if this.SpecZoneSel.Exclude.Has(id) {
				this.ZoneSel.Include.Delete(id)
				this.ZoneSel.Exclude.Insert(id)
			}
		}
	}
	for _, z := range zones {
		if this.ZoneSel.Include.Has(z.ZoneID().ID) {
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
			this.DomainSel.Include.Insert(z.Domain())
		}
	} else {
		if len(this.DomainSel.Include) == 0 {
			this.ZoneSel.Exclude.Insert(this.ZoneSel.Include.UnsortedList()...)
			this.ZoneSel.Include = sets.New[string]()
			zoneDomains := []string{}
			for _, z := range this.Zones {
				zoneDomains = append(zoneDomains, z.Domain())
			}
			this.Zones = nil
			this.Error = fmt.Sprintf("no domain matching hosting zones. Need to be a (sub)domain of [%s]",
				strings.Join(zoneDomains, ", "))
			for _, z := range allzones {
				this.DomainSel.Exclude.Insert(z.Domain())
			}
			return this
		}
	}

	zoneExcludeCandidates := sets.New[LightDNSHostedZone]()
outer:
	for _, zone := range this.Zones {
		for domain := range this.DomainSel.Include {
			if dnsutils.Match(domain, zone.Domain()) {
				isMatching := true
				for _, forwardedZone := range forwardedZones[zone.ZoneID().ID] {
					if dnsutils.Match(domain, forwardedZone.Domain()) {
						isMatching = false
						break
					}
				}
				if isMatching {
					continue outer
				}
			}
		}
		zoneExcludeCandidates.Insert(zone)
	}

	for id := range this.ZoneSel.Include {
		for _, forwardedZone := range forwardedZones[id] {
			isMatching := false
			for domain := range this.DomainSel.Include {
				if dnsutils.Match(forwardedZone.Domain(), domain) {
					isMatching = true
					break
				}
			}
			if isMatching {
				zoneExcludeCandidates.Delete(forwardedZone)
			}
		}
	}

outerExclude:
	for _, zone := range this.Zones {
		for domain := range this.DomainSel.Exclude {
			if dnsutils.Match(zone.Domain(), domain) {
				zoneExcludeCandidates.Insert(zone)
				continue outerExclude
			}
		}
	}

	for zone := range zoneExcludeCandidates {
		this.ZoneSel.Include.Delete(zone.ZoneID().ID)
		this.ZoneSel.Exclude.Insert(zone.ZoneID().ID)
	}

outerExcludeDomain:
	for _, z := range allzones {
		if !this.ZoneSel.Include.Has(z.ZoneID().ID) && !this.DomainSel.Include.Has(z.Domain()) {
			for domain := range this.DomainSel.Include {
				if dnsutils.Match(domain, z.Domain()) && domain != z.Domain() {
					continue outerExcludeDomain
				}
			}
			this.DomainSel.Exclude.Insert(z.Domain())
		}
	}
	if len(this.ZoneSel.Include) != len(this.Zones) {
		this.Zones = nil
		for _, z := range zones {
			if this.ZoneSel.Include.Has(z.ZoneID().ID) {
				this.Zones = append(this.Zones, z)
			}
		}
	}

	return this
}

func validateDomains(domains sets.Set[string], name string) error {
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
		subSel.Include = sets.New[string](sel.Include...)
		subSel.Exclude = sets.New[string](sel.Exclude...)
	}
	return subSel
}

func filterByZones(domains sets.Set[string], zones []LightDNSHostedZone) (result sets.Set[string], err error) {
	result = sets.Set[string]{}
	for d := range domains {
		for _, z := range zones {
			if dnsutils.Match(d, z.Domain()) {
				result.Insert(d)
			}
		}
		if !result.Has(d) {
			err = fmt.Errorf("domain %q not in hosted domains", d)
		}
	}
	return result, err
}

func normalizeDomains(domains sets.Set[string]) sets.Set[string] {
	if len(domains) == 0 {
		return domains
	}

	normalized := sets.New[string]()
	for k := range domains {
		k = dns.NormalizeDomainName(strings.ToLower(k))
		normalized.Insert(k)
	}
	return normalized
}
