// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package selection_test

import (
	"github.com/gardener/controller-manager-library/pkg/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
			Zones:       []LightDNSHostedZone{zab},
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("A.b."),
				Exclude: utils.NewStringSet("O.P."),
			},
			ZoneSel: SubSelection{
				Include: utils.NewStringSet("ZAB"),
				Exclude: utils.NewStringSet("ZCAB", "ZOP"),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet("a.b"),
				Exclude: utils.NewStringSet("c.a.b", "d.a.b", "o.p"),
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

	It("validates domain includes", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Domains: &v1alpha1.DNSSelection{
				Include: []string{"*.a.b"},
				Exclude: []string{"sub.a.b"},
			},
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("*.a.b"),
				Exclude: utils.NewStringSet("sub.a.b"),
			},
			ZoneSel:   NewSubSelection(),
			DomainSel: NewSubSelection(),
			Error:     "wildcards are not allowed in domains include '*.a.b' (hint: remove the wildcard)",
		}))
	})

	It("validates domain excludes", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Domains: &v1alpha1.DNSSelection{
				Include: []string{"a.b"},
				Exclude: []string{"*.sub.a.b"},
			},
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("a.b"),
				Exclude: utils.NewStringSet("*.sub.a.b"),
			},
			ZoneSel:   NewSubSelection(),
			DomainSel: NewSubSelection(),
			Error:     "wildcards are not allowed in domains exclude '*.sub.a.b' (hint: remove the wildcard)",
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
				Exclude: utils.NewStringSet("a.b", "o.p"),
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
				Exclude: utils.NewStringSet("c.a.b", "d.a.b", "o.p"),
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
			Zones:       []LightDNSHostedZone{zab},
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("a.b"),
				Exclude: utils.NewStringSet(),
			},
			ZoneSel: SubSelection{
				Include: utils.NewStringSet("ZAB"),
				Exclude: utils.NewStringSet("ZCAB", "ZOP"),
			},
			DomainSel: SubSelection{
				Include: utils.NewStringSet("a.b"),
				Exclude: utils.NewStringSet("c.a.b", "d.a.b", "o.p"),
			},
		}))
	})

	It("handles domain inclusion with inclusion of domain of sub hosted zone", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Domains: &v1alpha1.DNSSelection{
				Include: []string{"a.b", "c.a.b"},
				Exclude: []string{},
			},
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			Zones:       []LightDNSHostedZone{zab, zcab},
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: utils.NewStringSet("a.b", "c.a.b"),
				Exclude: utils.NewStringSet(),
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
				Exclude: utils.NewStringSet("a.b", "e.a.b", "y.x.o.p"),
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
				Exclude: utils.NewStringSet("a.b", "c.a.b", "o.p"),
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
				Exclude: utils.NewStringSet("c.a.b"),
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

	Context("forwarded own zones", func() {
		zb := &lightDNSHostedZone{
			id:               dns.NewZoneID("test", "ZB"),
			domain:           "b",
			forwardedDomains: []string{"c.a.b"},
		}
		It("do not include forwarded subsubdomain", func() {
			spec := v1alpha1.DNSProviderSpec{
				Type: "test",
				Domains: &v1alpha1.DNSSelection{
					Include: []string{"a.b"},
					Exclude: nil,
				},
			}
			result := CalcZoneAndDomainSelection(spec, []LightDNSHostedZone{zb, zcab})
			Expect(result).To(Equal(SelectionResult{
				Zones:       []LightDNSHostedZone{zb},
				SpecZoneSel: NewSubSelection(),
				SpecDomainSel: SubSelection{
					Include: utils.NewStringSet("a.b"),
					Exclude: utils.NewStringSet(),
				},
				ZoneSel: SubSelection{
					Include: utils.NewStringSet("ZB"),
					Exclude: utils.NewStringSet("ZCAB"),
				},
				DomainSel: SubSelection{
					Include: utils.NewStringSet("a.b"),
					Exclude: utils.NewStringSet("c.a.b"),
				},
			}))
		})
		It("do include subdomain of forwarded subsubdomain", func() {
			spec := v1alpha1.DNSProviderSpec{
				Type: "test",
				Domains: &v1alpha1.DNSSelection{
					Include: []string{"a.b", "d.c.a.b"},
					Exclude: nil,
				},
			}
			result := CalcZoneAndDomainSelection(spec, []LightDNSHostedZone{zb, zcab})
			Expect(result).To(Equal(SelectionResult{
				Zones:       []LightDNSHostedZone{zb, zcab},
				SpecZoneSel: NewSubSelection(),
				SpecDomainSel: SubSelection{
					Include: utils.NewStringSet("a.b", "d.c.a.b"),
					Exclude: utils.NewStringSet(),
				},
				ZoneSel: SubSelection{
					Include: utils.NewStringSet("ZB", "ZCAB"),
					Exclude: utils.NewStringSet(),
				},
				DomainSel: SubSelection{
					Include: utils.NewStringSet("a.b", "d.c.a.b"),
					Exclude: utils.NewStringSet("c.a.b"),
				},
			}))
		})
		It("use include subdomain of forwarded subsubdomain (2)", func() {
			spec := v1alpha1.DNSProviderSpec{
				Type: "test",
				Domains: &v1alpha1.DNSSelection{
					Include: []string{"b", "d.c.a.b"},
					Exclude: nil,
				},
			}
			result := CalcZoneAndDomainSelection(spec, []LightDNSHostedZone{zb, zcab})
			Expect(result).To(Equal(SelectionResult{
				Zones:       []LightDNSHostedZone{zb, zcab},
				SpecZoneSel: NewSubSelection(),
				SpecDomainSel: SubSelection{
					Include: utils.NewStringSet("b", "d.c.a.b"),
					Exclude: utils.NewStringSet(),
				},
				ZoneSel: SubSelection{
					Include: utils.NewStringSet("ZB", "ZCAB"),
					Exclude: utils.NewStringSet(),
				},
				DomainSel: SubSelection{
					Include: utils.NewStringSet("b", "d.c.a.b"),
					Exclude: utils.NewStringSet("c.a.b"),
				},
			}))
		})
		It("inconsistent zone and domain includes", func() {
			spec := v1alpha1.DNSProviderSpec{
				Type: "test",
				Domains: &v1alpha1.DNSSelection{
					Include: []string{"d.c.a.b"},
					Exclude: nil,
				},
				Zones: &v1alpha1.DNSSelection{
					Include: []string{"ZB"},
					Exclude: nil,
				},
			}
			result := CalcZoneAndDomainSelection(spec, []LightDNSHostedZone{zb, zcab})
			Expect(result).To(Equal(SelectionResult{
				Zones: nil,
				SpecZoneSel: SubSelection{
					Include: utils.NewStringSet("ZB"),
					Exclude: utils.NewStringSet(),
				},
				SpecDomainSel: SubSelection{
					Include: utils.NewStringSet("d.c.a.b"),
					Exclude: utils.NewStringSet(),
				},
				ZoneSel: SubSelection{
					Include: utils.NewStringSet(),
					Exclude: utils.NewStringSet("ZCAB", "ZB"),
				},
				DomainSel: SubSelection{
					Include: utils.NewStringSet(),
					Exclude: utils.NewStringSet("b", "c.a.b"),
				},
				Error: "no domain matching hosting zones. Need to be a (sub)domain of [b]",
				Warnings: []string{
					"domain \"d.c.a.b\" not in hosted domains",
				},
			}))
		})
	})
	Context("private zones", func() {
		zab2 := &lightDNSHostedZone{
			id:               dns.NewZoneID("test", "ZAB2"),
			domain:           "a.b",
			forwardedDomains: nil,
		}
		It("provides correct domain includes and excludes for private zones with same domain", func() {
			spec := v1alpha1.DNSProviderSpec{
				Type: "test",
				Zones: &v1alpha1.DNSSelection{
					Include: []string{"ZAB"},
					Exclude: []string{"ZAB2"},
				},
			}
			result := CalcZoneAndDomainSelection(spec, []LightDNSHostedZone{zab, zab2})
			Expect(result).To(Equal(SelectionResult{
				Zones: []LightDNSHostedZone{zab},
				SpecZoneSel: SubSelection{
					Include: utils.NewStringSet("ZAB"),
					Exclude: utils.NewStringSet("ZAB2"),
				},
				SpecDomainSel: SubSelection{
					Include: utils.NewStringSet(),
					Exclude: utils.NewStringSet(),
				},
				ZoneSel: SubSelection{
					Include: utils.NewStringSet("ZAB"),
					Exclude: utils.NewStringSet("ZAB2"),
				},
				DomainSel: SubSelection{
					Include: utils.NewStringSet("a.b"),
					Exclude: utils.NewStringSet("c.a.b", "d.a.b"),
				},
			}))
		})
	})
})
