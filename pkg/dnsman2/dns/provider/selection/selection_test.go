// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package selection_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	. "github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/selection"
)

type lightDNSHostedZone struct {
	id     dns.ZoneID
	domain string
}

func (z *lightDNSHostedZone) ZoneID() dns.ZoneID { return z.id }
func (z *lightDNSHostedZone) Domain() string     { return z.domain }

var _ = Describe("Selection", func() {
	zab := &lightDNSHostedZone{
		id:     dns.NewZoneID("test", "ZAB"),
		domain: "a.b",
	}
	zab2 := &lightDNSHostedZone{
		id:     dns.NewZoneID("test", "ZAB2"),
		domain: "a.b",
	}
	zcab := &lightDNSHostedZone{
		id:     dns.NewZoneID("test", "ZCAB"),
		domain: "c.a.b",
	}
	zfab := &lightDNSHostedZone{
		id:     dns.NewZoneID("test", "ZFAB"),
		domain: "f.a.b",
	}
	zop := &lightDNSHostedZone{
		id:     dns.NewZoneID("test", "ZOP"),
		domain: "o.p",
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
				Include: sets.New[string]("ZAB", "ZCAB", "ZOP"),
				Exclude: sets.New[string](),
			},
			DomainSel: SubSelection{
				Include: sets.New[string]("a.b", "c.a.b", "o.p"),
				Exclude: sets.New[string](),
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
				Include: sets.New[string]("A.b."),
				Exclude: sets.New[string]("O.P."),
			},
			ZoneSel: SubSelection{
				Include: sets.New[string]("ZAB", "ZCAB"),
				Exclude: sets.New[string]("ZOP"),
			},
			DomainSel: SubSelection{
				Include: sets.New[string]("a.b"),
				Exclude: sets.New[string]("o.p"),
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
				Include: sets.New[string](),
				Exclude: sets.New[string](),
			},
			DomainSel: SubSelection{
				Include: sets.New[string](),
				Exclude: sets.New[string](),
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
				Include: sets.New[string]("*.a.b"),
				Exclude: sets.New[string]("sub.a.b"),
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
				Include: sets.New[string]("a.b"),
				Exclude: sets.New[string]("*.sub.a.b"),
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
				Include: sets.New[string](),
				Exclude: sets.New[string]("ZAB", "ZOP"),
			},
			SpecDomainSel: NewSubSelection(),
			ZoneSel: SubSelection{
				Include: sets.New[string]("ZCAB"),
				Exclude: sets.New[string]("ZAB", "ZOP"),
			},
			DomainSel: SubSelection{
				Include: sets.New[string]("c.a.b"),
				Exclude: sets.New[string]("o.p"),
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
				Include: sets.New[string]("ZAB"),
				Exclude: sets.New[string]("ZOP"),
			},
			SpecDomainSel: NewSubSelection(),
			ZoneSel: SubSelection{
				Include: sets.New[string]("ZAB"),
				Exclude: sets.New[string]("ZCAB", "ZOP"),
			},
			DomainSel: SubSelection{
				Include: sets.New[string]("a.b"),
				Exclude: sets.New[string]("c.a.b", "o.p"),
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
				Include: sets.New[string]("a.b"),
				Exclude: sets.New[string](),
			},
			ZoneSel: SubSelection{
				Include: sets.New[string]("ZAB", "ZCAB"),
				Exclude: sets.New[string]("ZOP"),
			},
			DomainSel: SubSelection{
				Include: sets.New[string]("a.b"),
				Exclude: sets.New[string]("o.p"),
			},
		}))
	})

	It("handles domain inclusion with exclusion of forwarded zone", func() {
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
				Include: sets.New[string]("a.b"),
				Exclude: sets.New[string]("c.a.b"),
			},
			ZoneSel: SubSelection{
				Include: sets.New[string]("ZAB"),
				Exclude: sets.New[string]("ZOP", "ZCAB"),
			},
			DomainSel: SubSelection{
				Include: sets.New[string]("a.b"),
				Exclude: sets.New[string]("c.a.b", "o.p"),
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
				Include: sets.New[string]("a.b", "c.a.b"),
				Exclude: sets.New[string](),
			},
			ZoneSel: SubSelection{
				Include: sets.New[string]("ZAB", "ZCAB"),
				Exclude: sets.New[string]("ZOP"),
			},
			DomainSel: SubSelection{
				Include: sets.New[string]("a.b", "c.a.b"),
				Exclude: sets.New[string]("o.p"),
			},
		}))
	})

	It("handles complex domain inclusion", func() {
		spec := v1alpha1.DNSProviderSpec{
			Type: "test",
			Domains: &v1alpha1.DNSSelection{
				Include: []string{"c.a.b", "x.o.p"},
				Exclude: []string{"e.a.b", "y.x.o.p"},
			},
		}
		result := CalcZoneAndDomainSelection(spec, allzones)
		Expect(result).To(Equal(SelectionResult{
			Zones:       []LightDNSHostedZone{zcab, zop},
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: sets.New[string]("c.a.b", "x.o.p"),
				Exclude: sets.New[string]("e.a.b", "y.x.o.p"),
			},
			ZoneSel: SubSelection{
				Include: sets.New[string]("ZCAB", "ZOP"),
				Exclude: sets.New[string]("ZAB"),
			},
			DomainSel: SubSelection{
				Include: sets.New[string]("c.a.b", "x.o.p"),
				Exclude: sets.New[string]("e.a.b", "y.x.o.p"),
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
				Include: sets.New[string]("y.z"),
				Exclude: sets.New[string](),
			},
			ZoneSel: SubSelection{
				Include: sets.New[string](),
				Exclude: sets.New[string]("ZAB", "ZCAB", "ZOP"),
			},
			DomainSel: SubSelection{
				Include: sets.New[string](),
				Exclude: sets.New[string]("a.b", "c.a.b", "o.p"),
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
				Include: sets.New[string]("f.a.b"),
				Exclude: sets.New[string](),
			},
			ZoneSel: SubSelection{
				Include: sets.New[string]("ZAB", "ZAB2"),
				Exclude: sets.New[string]("ZCAB"),
			},
			DomainSel: SubSelection{
				Include: sets.New[string]("f.a.b"),
				Exclude: sets.New[string]("c.a.b"),
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
			Zones:       []LightDNSHostedZone{zfab},
			SpecZoneSel: NewSubSelection(),
			SpecDomainSel: SubSelection{
				Include: sets.New[string]("d.f.a.b"),
				Exclude: sets.New[string](),
			},
			ZoneSel: SubSelection{
				Include: sets.New[string]("ZFAB"),
				Exclude: sets.New[string]("ZAB"),
			},
			DomainSel: SubSelection{
				Include: sets.New[string]("d.f.a.b"),
				Exclude: sets.New[string](),
			},
		}))
	})

	Context("forwarded own zones", func() {
		zb := &lightDNSHostedZone{
			id:     dns.NewZoneID("test", "ZB"),
			domain: "b",
		}
		It("includes forwarded subsubdomain", func() {
			spec := v1alpha1.DNSProviderSpec{
				Type: "test",
				Domains: &v1alpha1.DNSSelection{
					Include: []string{"a.b"},
					Exclude: nil,
				},
			}
			result := CalcZoneAndDomainSelection(spec, []LightDNSHostedZone{zb, zcab})
			Expect(result).To(Equal(SelectionResult{
				Zones:       []LightDNSHostedZone{zb, zcab},
				SpecZoneSel: NewSubSelection(),
				SpecDomainSel: SubSelection{
					Include: sets.New[string]("a.b"),
					Exclude: sets.New[string](),
				},
				ZoneSel: SubSelection{
					Include: sets.New[string]("ZB", "ZCAB"),
					Exclude: sets.New[string](),
				},
				DomainSel: SubSelection{
					Include: sets.New[string]("a.b"),
					Exclude: sets.New[string](),
				},
			}))
		})
		It("includes subdomain of forwarded subsubdomain", func() {
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
					Include: sets.New[string]("a.b", "d.c.a.b"),
					Exclude: sets.New[string](),
				},
				ZoneSel: SubSelection{
					Include: sets.New[string]("ZB", "ZCAB"),
					Exclude: sets.New[string](),
				},
				DomainSel: SubSelection{
					Include: sets.New[string]("a.b", "d.c.a.b"),
					Exclude: sets.New[string](),
				},
			}))
		})
		It("includes subdomain of forwarded subsubdomain (2)", func() {
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
					Include: sets.New[string]("b", "d.c.a.b"),
					Exclude: sets.New[string](),
				},
				ZoneSel: SubSelection{
					Include: sets.New[string]("ZB", "ZCAB"),
					Exclude: sets.New[string](),
				},
				DomainSel: SubSelection{
					Include: sets.New[string]("b", "d.c.a.b"),
					Exclude: sets.New[string](),
				},
			}))
		})
		It("handles inconsistent zone and domain includes", func() {
			spec := v1alpha1.DNSProviderSpec{
				Type: "test",
				Domains: &v1alpha1.DNSSelection{
					Include: []string{"d.c.b.a"},
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
					Include: sets.New[string]("ZB"),
					Exclude: sets.New[string](),
				},
				SpecDomainSel: SubSelection{
					Include: sets.New[string]("d.c.b.a"),
					Exclude: sets.New[string](),
				},
				ZoneSel: SubSelection{
					Include: sets.New[string](),
					Exclude: sets.New[string]("ZCAB", "ZB"),
				},
				DomainSel: SubSelection{
					Include: sets.New[string](),
					Exclude: sets.New[string]("b", "c.a.b"),
				},
				Error: "no domain matching hosting zones. Need to be a (sub)domain of [b]",
				Warnings: []string{
					"domain \"d.c.b.a\" not in hosted domains",
				},
			}))
		})
	})
	Context("private zones", func() {
		zab2 := &lightDNSHostedZone{
			id:     dns.NewZoneID("test", "ZAB2"),
			domain: "a.b",
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
					Include: sets.New[string]("ZAB"),
					Exclude: sets.New[string]("ZAB2"),
				},
				SpecDomainSel: SubSelection{
					Include: sets.New[string](),
					Exclude: sets.New[string](),
				},
				ZoneSel: SubSelection{
					Include: sets.New[string]("ZAB"),
					Exclude: sets.New[string]("ZAB2"),
				},
				DomainSel: SubSelection{
					Include: sets.New[string]("a.b"),
					Exclude: sets.New[string](),
				},
			}))
		})
	})
})
