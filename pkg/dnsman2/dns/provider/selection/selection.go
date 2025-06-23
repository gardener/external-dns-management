// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package selection

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// SelectionResult contains the result of the CalcZoneAndDomainSelection function.
type SelectionResult struct {
	// Zones is the list of selected DNS hosted zones.
	Zones []LightDNSHostedZone
	// SpecZoneSel is the zone selection from the provider spec.
	SpecZoneSel SubSelection
	// SpecDomainSel is the domain selection from the provider spec.
	SpecDomainSel SubSelection
	// ZoneSel is the effective zone selection after processing.
	ZoneSel SubSelection
	// DomainSel is the effective domain selection after processing.
	DomainSel SubSelection
	// Error contains an error message if selection failed.
	Error string
	// Warnings contains warning messages encountered during selection.
	Warnings []string
}

// SetProviderStatusZonesAndDomains sets the included and excluded zones and domains in the provider status.
func (r SelectionResult) SetProviderStatusZonesAndDomains(status *v1alpha1.DNSProviderStatus) {
	status.Zones = v1alpha1.DNSSelectionStatus{Included: toSortedList(r.ZoneSel.Include), Excluded: toSortedList(r.ZoneSel.Exclude)}
	status.Domains = v1alpha1.DNSSelectionStatus{Included: toSortedList(r.DomainSel.Include), Excluded: toSortedList(r.DomainSel.Exclude)}
}

// SubSelection contains an included and an excluded string set.
type SubSelection struct {
	Include sets.Set[string]
	Exclude sets.Set[string]
}

// NewSubSelection creates an empty SubSelection.
func NewSubSelection() SubSelection {
	return SubSelection{
		Include: sets.New[string](),
		Exclude: sets.New[string](),
	}
}

// LightDNSHostedZone contains the info of a DNSHostedZone needed for selection.
type LightDNSHostedZone interface {
	// ZoneID returns the zone ID of the hosted zone.
	ZoneID() dns.ZoneID
	// Domain returns the domain name of the hosted zone.
	Domain() string
}

// CalcZoneAndDomainSelection calculates the effective included/excluded domains and zones for the given spec and
// zones supported by a provider.
func CalcZoneAndDomainSelection(spec v1alpha1.DNSProviderSpec, allzones []LightDNSHostedZone) SelectionResult {
	result := SelectionResult{
		SpecDomainSel: PrepareSelection(spec.Domains),
		SpecZoneSel:   PrepareSelection(spec.Zones),
		ZoneSel:       NewSubSelection(),
		DomainSel:     NewSubSelection(),
	}

	if err := validateDomains(result.SpecDomainSel.Include, "domains include"); err != nil {
		result.Error = err.Error()
		return result
	}
	if err := validateDomains(result.SpecDomainSel.Exclude, "domains exclude"); err != nil {
		result.Error = err.Error()
		return result
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

	if len(result.SpecZoneSel.Include) > 0 {
		for _, z := range zones {
			if result.SpecZoneSel.Include.Has(z.ZoneID().ID) {
				result.ZoneSel.Include.Insert(z.ZoneID().ID)
			} else {
				result.ZoneSel.Exclude.Insert(z.ZoneID().ID)
			}
		}
	} else {
		for _, z := range zones {
			result.ZoneSel.Include.Insert(z.ZoneID().ID)
		}
	}
	if len(result.SpecZoneSel.Exclude) > 0 {
		for id := range result.ZoneSel.Include {
			if result.SpecZoneSel.Exclude.Has(id) {
				result.ZoneSel.Include.Delete(id)
				result.ZoneSel.Exclude.Insert(id)
			}
		}
	}
	for _, z := range zones {
		if result.ZoneSel.Include.Has(z.ZoneID().ID) {
			result.Zones = append(result.Zones, z)
		}
	}

	if len(zones) > 0 && len(result.Zones) == 0 {
		result.Error = "no zone available in account matches zone filter"
		return result
	}

	var err error
	result.DomainSel.Include, err = filterByZones(normalizeDomains(result.SpecDomainSel.Include), result.Zones)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	result.DomainSel.Exclude, err = filterByZones(normalizeDomains(result.SpecDomainSel.Exclude), result.Zones)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}

	if len(result.SpecDomainSel.Include) == 0 {
		if len(result.Zones) == 0 {
			result.Error = "no hosted zones found"
			return result
		}
		for _, z := range result.Zones {
			result.DomainSel.Include.Insert(z.Domain())
		}
	} else {
		if len(result.DomainSel.Include) == 0 {
			result.ZoneSel.Exclude.Insert(result.ZoneSel.Include.UnsortedList()...)
			result.ZoneSel.Include = sets.New[string]()
			zoneDomains := []string{}
			for _, z := range result.Zones {
				zoneDomains = append(zoneDomains, z.Domain())
			}
			result.Zones = nil
			result.Error = fmt.Sprintf("no domain matching hosting zones. Need to be a (sub)domain of [%s]",
				strings.Join(zoneDomains, ", "))
			for _, z := range allzones {
				result.DomainSel.Exclude.Insert(z.Domain())
			}
			return result
		}
	}

	zoneExcludeCandidates := sets.New[LightDNSHostedZone]()
outer:
	for _, zone := range result.Zones {
		for domain := range result.DomainSel.Include {
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

	for id := range result.ZoneSel.Include {
		for _, forwardedZone := range forwardedZones[id] {
			isMatching := false
			for domain := range result.DomainSel.Include {
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
	for _, zone := range result.Zones {
		for domain := range result.DomainSel.Exclude {
			if dnsutils.Match(zone.Domain(), domain) {
				zoneExcludeCandidates.Insert(zone)
				continue outerExclude
			}
		}
	}

	for zone := range zoneExcludeCandidates {
		result.ZoneSel.Include.Delete(zone.ZoneID().ID)
		result.ZoneSel.Exclude.Insert(zone.ZoneID().ID)
	}

outerExcludeDomain:
	for _, z := range allzones {
		if !result.ZoneSel.Include.Has(z.ZoneID().ID) && !result.DomainSel.Include.Has(z.Domain()) {
			for domain := range result.DomainSel.Include {
				if dnsutils.Match(domain, z.Domain()) && domain != z.Domain() {
					continue outerExcludeDomain
				}
			}
			result.DomainSel.Exclude.Insert(z.Domain())
		}
	}
	if len(result.ZoneSel.Include) != len(result.Zones) {
		result.Zones = nil
		for _, z := range zones {
			if result.ZoneSel.Include.Has(z.ZoneID().ID) {
				result.Zones = append(result.Zones, z)
			}
		}
	}

	return result
}

func validateDomains(domains sets.Set[string], name string) error {
	for domain := range domains {
		if strings.HasPrefix(domain, "*.") {
			return fmt.Errorf("wildcards are not allowed in %s '%s' (hint: remove the wildcard)", name, domain)
		}
	}
	return nil
}

// PrepareSelection creates a SubSelection from a DNSSelection.
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

func toSortedList(set sets.Set[string]) []string {
	if len(set) == 0 {
		return nil
	}
	list := set.UnsortedList()
	sort.Strings(list)
	return list
}
