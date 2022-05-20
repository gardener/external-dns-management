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

package selection_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	. "github.com/gardener/external-dns-management/pkg/dns/provider/selection"
)

type lightDNSHostedZone struct {
	id               dns.ZoneID
	domain           string
	forwardedDomains []string
}

func (z *lightDNSHostedZone) Id() dns.ZoneID             { return z.id }
func (z *lightDNSHostedZone) Domain() string             { return z.domain }
func (z *lightDNSHostedZone) ForwardedDomains() []string { return z.forwardedDomains }

var _ = Describe("Selection", func() {
	zab := &lightDNSHostedZone{
		id:               dns.NewZoneID("test", "ZAB"),
		domain:           "a.b",
		forwardedDomains: []string{"c.a.b", "d.a.b"},
	}
	zab2 := &lightDNSHostedZone{
		id:               dns.NewZoneID("test", "ZAB2"),
		domain:           "a.b",
		forwardedDomains: []string{},
	}
	zcab := &lightDNSHostedZone{
		id:               dns.NewZoneID("test", "ZCAB"),
		domain:           "c.a.b",
		forwardedDomains: nil,
	}
	zfab := &lightDNSHostedZone{
		id:               dns.NewZoneID("test", "ZFAB"),
		domain:           "f.a.b",
		forwardedDomains: nil,
	}
	zop := &lightDNSHostedZone{
		id:               dns.NewZoneID("test", "ZOP"),
		domain:           "o.p",
		forwardedDomains: nil,
	}
	nozones := []LightDNSHostedZone{}
	allzones := []LightDNSHostedZone{zab, zcab, zop}

	It("uses all zones if no spec given", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			Zones:         allzones,
			SpecZoneSel:   NewSubSelection(),
			SpecDomainSel: NewSubSelection(),
			ZoneSel: SubSelection{
				Include: utils.NewStringSet("ZAB", "ZCAB", "ZOP"),
				Exclude: utils.NewStringSet(),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet("a.b", "c.a.b", "o.p"),
				Exclude: utils.NewStringSet("d.a.b"),
			},
		}))
	})

	It("deals with uppercase domain selection and final dot", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Domains: &v1alpha1.DNSSelection{
				Include: []string{"A.b."},
				Exclude: []string{"O.P."},
			},
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			Zones:       []LightDNSHostedZone{zab, zcab},
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("A.b."),
				Exclude: utils.NewStringSet("O.P."),
			},
			ZoneSel: SubSelection{
				Include: utils.NewStringSet("ZAB", "ZCAB"),
				Exclude: utils.NewStringSet("ZOP"),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet("a.b", "c.a.b"),
				Exclude: utils.NewStringSet("d.a.b", "o.p"),
			},
		}))
	})

	It("handles no zones", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
		}
		result := CalcZoneAndDomainSelection(spec, nozones)
		Expect(result).To(Equal(SelectionResult{
			Zones:         nil,
			SpecZoneSel:   NewSubSelection(),
			SpecDomainSel: NewSubSelection(),
			ZoneSel: SubSelection{
				Include: utils.NewStringSet(),
				Exclude: utils.NewStringSet(),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet(),
				Exclude: utils.NewStringSet(),
			},
			Error: "no hosted zones found",
		}))
	})

	It("handles zones exclusion", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Zones: &v1alpha1.DNSSelection{
				Include: nil,
				Exclude: []string{"ZOP", "ZAB"},
			},
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			Zones: []LightDNSHostedZone{zcab},
			SpecZoneSel: SubSelection{
				Include: utils.NewStringSet(),
				Exclude: utils.NewStringSet("ZAB", "ZOP"),
			},
			SpecDomainSel: NewSubSelection(),
			ZoneSel: SubSelection{
				Include: utils.NewStringSet("ZCAB"),
				Exclude: utils.NewStringSet("ZAB", "ZOP"),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet("c.a.b"),
				Exclude: utils.NewStringSet(),
			},
		}))
	})

	It("handles zones inclusion", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Zones: &v1alpha1.DNSSelection{
				Include: []string{"ZAB"},
				Exclude: []string{"ZOP"},
			},
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			Zones: []LightDNSHostedZone{zab},
			SpecZoneSel: SubSelection{
				Include: utils.NewStringSet("ZAB"),
				Exclude: utils.NewStringSet("ZOP"),
			},
			SpecDomainSel: NewSubSelection(),
			ZoneSel: SubSelection{
				Include: utils.NewStringSet("ZAB"),
				Exclude: utils.NewStringSet("ZCAB", "ZOP"),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet("a.b"),
				Exclude: utils.NewStringSet("c.a.b", "d.a.b"),
			},
		}))
	})

	It("handles simple domain inclusion", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Domains: &v1alpha1.DNSSelection{
				Include: []string{"a.b"},
				Exclude: nil,
			},
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			Zones:       []LightDNSHostedZone{zab, zcab},
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("a.b"),
				Exclude: utils.NewStringSet(),
			},
			ZoneSel: SubSelection{
				Include: utils.NewStringSet("ZAB", "ZCAB"),
				Exclude: utils.NewStringSet("ZOP"),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet("a.b", "c.a.b"),
				Exclude: utils.NewStringSet("d.a.b"),
			},
		}))
	})

	It("handles domain inclusion with exclusion of domain of sub hosted zone", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Domains: &v1alpha1.DNSSelection{
				Include: []string{"a.b"},
				Exclude: []string{"c.a.b"},
			},
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			Zones:       []LightDNSHostedZone{zab},
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("a.b"),
				Exclude: utils.NewStringSet("c.a.b"),
			},
			ZoneSel: SubSelection{
				Include: utils.NewStringSet("ZAB"),
				Exclude: utils.NewStringSet("ZCAB", "ZOP"),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet("a.b"),
				Exclude: utils.NewStringSet("c.a.b", "d.a.b"),
			},
		}))
	})

	It("handles complex domain inclusion", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Domains: &v1alpha1.DNSSelection{
				Include: []string{"c.a.b", "x.o.p"},
				Exclude: []string{"d.a.b", "e.a.b", "y.x.o.p"},
			},
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			Zones:       []LightDNSHostedZone{zcab, zop},
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("c.a.b", "x.o.p"),
				Exclude: utils.NewStringSet("d.a.b", "e.a.b", "y.x.o.p"),
			},
			ZoneSel: SubSelection{
				Include: utils.NewStringSet("ZCAB", "ZOP"),
				Exclude: utils.NewStringSet("ZAB"),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet("c.a.b", "x.o.p"),
				Exclude: utils.NewStringSet("e.a.b", "y.x.o.p"),
			},
			Warnings: []string{
				"domain \"d.a.b\" not in hosted domains",
			},
		}))
	})

	It("handles foreign domain inclusion", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Domains: &v1alpha1.DNSSelection{
				Include: []string{"y.z"},
				Exclude: nil,
			},
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			Zones:       nil,
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("y.z"),
				Exclude: utils.NewStringSet(),
			},
			ZoneSel: SubSelection{
				Include: utils.NewStringSet(),
				Exclude: utils.NewStringSet("ZAB", "ZCAB", "ZOP"),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet(),
				Exclude: utils.NewStringSet(),
			},
			Error: "no domain matching hosting zones. Need to be a (sub)domain of [a.b, c.a.b, o.p]",
			Warnings: []string{
				"domain \"y.z\" not in hosted domains",
			},
		}))
	})

	It("matches duplicate zones with same base domain by domain inclusion", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Domains: &v1alpha1.DNSSelection{
				Include: []string{"f.a.b"},
				Exclude: nil,
			},
		}
		result := CalcZoneAndDomainSelection(spec, []LightDNSHostedZone{zab, zab2, zcab})
		Expect(result).To(Equal(SelectionResult{
			Zones:       []LightDNSHostedZone{zab, zab2},
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("f.a.b"),
				Exclude: utils.NewStringSet(),
			},
			ZoneSel: SubSelection{
				Include: utils.NewStringSet("ZAB", "ZAB2"),
				Exclude: utils.NewStringSet("ZCAB"),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet("f.a.b"),
				Exclude: utils.NewStringSet(),
			},
		}))
	})

	It("matches duplicate zones with overlapping base domain by domain inclusion", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Domains: &v1alpha1.DNSSelection{
				Include: []string{"d.f.a.b"},
				Exclude: nil,
			},
		}
		result := CalcZoneAndDomainSelection(spec, []LightDNSHostedZone{zab, zfab})
		Expect(result).To(Equal(SelectionResult{
			Zones:       []LightDNSHostedZone{zab, zfab},
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("d.f.a.b"),
				Exclude: utils.NewStringSet(),
			},
			ZoneSel: SubSelection{
				Include: utils.NewStringSet("ZAB", "ZFAB"),
				Exclude: utils.NewStringSet(),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet("d.f.a.b"),
				Exclude: utils.NewStringSet(),
			},
		}))
	})

})
