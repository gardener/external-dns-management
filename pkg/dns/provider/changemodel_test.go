package provider

import (
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type simpleSpec struct{ targets []dnsutils.Target }

func (s *simpleSpec) Targets() []dnsutils.Target        { return s.targets }
func (s *simpleSpec) RoutingPolicy() *dns.RoutingPolicy { return nil }

func chosenProviderHash(m *ChangeModel, name dns.DNSSetName) (string, bool) {
	for hash, g := range m.providergroups {
		for _, r := range g.requests {
			if r.Addition != nil && r.Addition.Name == name {
				return hash, true
			}
		}
	}
	return "", false
}

type domainProvider struct {
	namespace string
	name      string
	hash      string
	includes  []string
	createdAt time.Time
}

func newDomainProvider(namespace, name, hash string, includes []string, created time.Time) *domainProvider {
	return &domainProvider{namespace: namespace, name: name, hash: hash, includes: includes, createdAt: created}
}

func (p *domainProvider) ObjectName() resources.ObjectName {
	return resources.NewObjectName(p.namespace, p.name)
}
func (p *domainProvider) Object() resources.Object                         { return nil }
func (p *domainProvider) TypeCode() string                                 { return "test" }
func (p *domainProvider) DefaultTTL() int64                                { return 300 }
func (p *domainProvider) GetZones() DNSHostedZones                         { return nil }
func (p *domainProvider) IncludesZone(dns.ZoneID) bool                     { return true }
func (p *domainProvider) HasEquivalentZone(dns.ZoneID) bool                { return false }
func (p *domainProvider) GetZoneState(DNSHostedZone) (DNSZoneState, error) { return nil, nil }
func (p *domainProvider) ExecuteRequests(logger.LogContext, DNSHostedZone, DNSZoneState, []*ChangeRequest) error {
	return nil
}
func (p *domainProvider) GetDedicatedDNSAccess() DedicatedDNSAccess { return nil }

func (p *domainProvider) Match(dnsName string) int {
	best := 0
	for _, suf := range p.includes {
		if strings.HasSuffix(dnsName, suf) {
			if l := len(suf); l > best {
				best = l
			}
		}
	}
	return best
}
func (p *domainProvider) MatchZone(string) int                                       { return 0 }
func (p *domainProvider) IsValid() bool                                              { return true }
func (p *domainProvider) AccountHash() string                                        { return p.hash }
func (p *domainProvider) UpdateGroup() string                                        { return p.ObjectName().Namespace() }
func (p *domainProvider) MapTargets(_ string, t []dnsutils.Target) []dnsutils.Target { return t }

type tinyZone struct {
	id     dns.ZoneID
	domain string
}

func (z *tinyZone) Key() string                { return z.id.ID }
func (z *tinyZone) Id() dns.ZoneID             { return z.id }
func (z *tinyZone) Domain() string             { return z.domain }
func (z *tinyZone) ForwardedDomains() []string { return nil }
func (z *tinyZone) Match(_ string) int         { return 1 }
func (z *tinyZone) IsPrivate() bool            { return false }

var _ = ginkgo.Describe("ChangeModel Provider Selection (Provenance)", func() {
	var (
		log    logger.LogContext
		proj1  *domainProvider
		proj2  *domainProvider
		proj3  *domainProvider
		model  *ChangeModel
		target dnsutils.TargetSpec
	)

	ginkgo.BeforeEach(func() {
		log = logger.New()
		now := time.Now()
		// Both providers include the SAME zone domain => identical Match() result for test names.
		proj1 = newDomainProvider("ns1", "provider-project1", "aaa111", []string{"example.test"}, now.Add(-2*time.Hour))
		proj2 = newDomainProvider("ns2", "provider-project2", "zzz999", []string{"example.test"}, now)
		proj3 = newDomainProvider("ns3", "provider-project3", "aaa111", []string{"example.test"}, now)

		zone := newDNSHostedZone(0, &tinyZone{
			id:     dns.NewZoneID("generic", "zone-01"),
			domain: "example.test",
		})

		rec := &zoneReconciliation{
			zone:      zone,
			providers: DNSProviders{proj1.ObjectName(): proj1, proj2.ObjectName(): proj2},
		}

		model = NewChangeModel(log, rec, Config{}, dns.DNSSets{})
		model.dangling = newChangeGroup("dangling", proj1, model)

		target = &simpleSpec{
			targets: []dnsutils.Target{dnsutils.NewTarget("A", "192.0.2.10", 120)},
		}
	})

	ginkgo.It("Don't perform updates with random provider, but ensure that same account is used if backend stores provenance information", func() {
		nameProject1 := dns.DNSSetName{DNSName: "project1-service.example.test"}
		nameProject2 := dns.DNSSetName{DNSName: "project2-service.example.test"}
		nameProject3 := dns.DNSSetName{DNSName: "project3-service.example.test"}

		Expect(model.Apply(nameProject1, "ns1", nil, target).Error).ToNot(HaveOccurred())
		Expect(model.Apply(nameProject2, "ns2", nil, target).Error).ToNot(HaveOccurred())
		Expect(model.Apply(nameProject3, "ns3", nil, target).Error).ToNot(HaveOccurred())

		hash1, ok1 := chosenProviderHash(model, nameProject1)
		hash2, ok2 := chosenProviderHash(model, nameProject2)
		hash3, ok3 := chosenProviderHash(model, nameProject3)

		Expect(ok1).To(BeTrue(), "project1 name queued")
		Expect(ok2).To(BeTrue(), "project2 name queued")
		Expect(ok3).To(BeTrue(), "project3 name queued")

		Expect(hash1).To(Equal(proj1.AccountHash()))
		Expect(hash2).To(Equal(proj2.AccountHash()))
		Expect(hash3).To(Equal(proj3.AccountHash()))
		Expect(hash3).To(Equal(hash1))

		Expect(model.providergroups).To(HaveLen(2))

		group := model.providergroups[hash1]
		var additions []string
		for _, r := range group.requests {
			if r.Addition != nil {
				additions = append(additions, r.Addition.Name.DNSName)
			}
		}
		Expect(additions).To(ContainElements(
			nameProject1.DNSName,
			nameProject3.DNSName,
		))
		group2 := model.providergroups[hash2]
		var additions2 []string
		for _, r := range group2.requests {
			if r.Addition != nil {
				additions2 = append(additions2, r.Addition.Name.DNSName)
			}
		}
		Expect(additions2).To(ContainElements(
			nameProject2.DNSName,
		))
	})
})
