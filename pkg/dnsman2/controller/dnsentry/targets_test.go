// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

const (
	ipv4Address  = "1.2.3.4"
	ipv4Address2 = "5.6.7.8"
	ipv6Address  = "2001:0db8:85a3::8a2e:0370:7334"
)

var _ = Describe("SpecToTargets", func() {
	var (
		key  client.ObjectKey
		spec *v1alpha1.DNSEntrySpec
	)

	BeforeEach(func() {
		spec = &v1alpha1.DNSEntrySpec{
			DNSName: "foo.example.com",
		}
		key = client.ObjectKey{Name: "test", Namespace: "default"}
	})

	It("returns targets for valid A record", func() {
		spec.Targets = []string{ipv4Address}

		targets, warnings, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).ToNot(HaveOccurred())
		Expect(warnings).To(BeEmpty())
		Expect(targets).To(HaveLen(1))
		Expect(targets[0].GetRecordType()).To(Equal(dns.TypeA))
		Expect(targets[0].GetTTL()).To(Equal(int64(360)))
		Expect(targets[0].GetRecordValue()).To(Equal(ipv4Address))
	})

	It("returns targets for valid AAAA record", func() {
		spec.Targets = []string{ipv6Address}
		spec.TTL = ptr.To[int64](120)

		targets, warnings, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).ToNot(HaveOccurred())
		Expect(warnings).To(BeEmpty())
		Expect(targets).To(HaveLen(1))
		Expect(targets[0].GetRecordType()).To(Equal(dns.TypeAAAA))
		Expect(targets[0].GetTTL()).To(Equal(int64(120)))
		Expect(targets[0].GetRecordValue()).To(Equal(ipv6Address))
	})

	It("returns targets for valid A and AAAA records", func() {
		spec.Targets = []string{ipv4Address, ipv4Address2, ipv6Address}

		targets, warnings, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).ToNot(HaveOccurred())
		Expect(warnings).To(BeEmpty())
		Expect(targets).To(HaveLen(3))
		Expect(targets[0].GetRecordType()).To(Equal(dns.TypeA))
		Expect(targets[0].GetRecordValue()).To(Equal(ipv4Address))
		Expect(targets[1].GetRecordType()).To(Equal(dns.TypeA))
		Expect(targets[1].GetRecordValue()).To(Equal(ipv4Address2))
		Expect(targets[2].GetRecordType()).To(Equal(dns.TypeAAAA))
		Expect(targets[2].GetRecordValue()).To(Equal(ipv6Address))
	})

	It("returns targets for valid CNAME record", func() {
		spec.Targets = []string{"*.example.com"}

		targets, warnings, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).ToNot(HaveOccurred())
		Expect(warnings).To(BeEmpty())
		Expect(targets).To(HaveLen(1))
		Expect(targets[0].GetRecordType()).To(Equal(dns.TypeCNAME))
		Expect(targets[0].GetRecordValue()).To(Equal("*.example.com"))
	})

	It("returns targets for valid TXT record", func() {
		spec.Text = []string{"example.com", ipv4Address}

		targets, warnings, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).ToNot(HaveOccurred())
		Expect(warnings).To(BeEmpty())
		Expect(targets).To(HaveLen(2))
		Expect(targets[0].GetRecordType()).To(Equal(dns.TypeTXT))
		Expect(targets[0].GetRecordValue()).To(Equal("example.com"))
		Expect(targets[1].GetRecordType()).To(Equal(dns.TypeTXT))
		Expect(targets[1].GetRecordValue()).To(Equal("1.2.3.4"))
	})

	It("returns warning for duplicate targets", func() {
		spec.Targets = []string{ipv4Address, ipv4Address}

		targets, warnings, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).ToNot(HaveOccurred())
		Expect(warnings).To(HaveLen(1))
		Expect(targets).To(HaveLen(1))
	})

	It("returns error for empty target", func() {
		spec.Targets = []string{""}

		_, _, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).To(MatchError(ContainSubstring("must not be empty")))
	})

	It("returns warning for empty text", func() {
		spec.Text = []string{""}

		_, warnings, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).To(MatchError(ContainSubstring("only empty text")))
		Expect(warnings).To(HaveLen(1))
	})

	It("returns error if both Targets and Text are set", func() {
		spec.Targets = []string{ipv4Address}
		spec.Text = []string{"foo"}

		_, _, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).To(MatchError(ContainSubstring("only text or targets possible")))
	})

	It("returns error if no targets or text are set", func() {
		_, _, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).To(MatchError(ContainSubstring("no target or text specified")))
	})

	It("returns error if there are multiple CNAME targets", func() {
		spec.Targets = []string{"foo.example.com", "bar.example.com"}
		_, _, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).To(MatchError(ContainSubstring("cannot have multiple CNAME targets")))
	})

	It("returns error if there are CNAME target is mixed with IP addresses", func() {
		spec.Targets = []string{"foo.example.com", ipv4Address}
		_, _, err := dnsentry.SpecToTargets(key, spec, "", 360)
		Expect(err).To(MatchError(ContainSubstring("cannot mix CNAME and other record types in targets")))
	})
})

var _ = Describe("StatusToTargets", func() {
	It("returns empty targets if status.Zone is nil", func() {
		status := &v1alpha1.DNSEntryStatus{}
		targets, err := dnsentry.StatusToTargets(status, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(targets).To(BeEmpty())
	})

	It("returns A and AAAA targets for valid IPs", func() {
		status := &v1alpha1.DNSEntryStatus{
			Zone:    ptr.To("zone1"),
			Targets: []string{ipv4Address, ipv6Address},
			TTL:     ptr.To[int64](120),
		}
		targets, err := dnsentry.StatusToTargets(status, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(targets).To(HaveLen(2))
		Expect(targets[0].GetRecordType()).To(Equal(dns.TypeA))
		Expect(targets[0].GetRecordValue()).To(Equal(ipv4Address))
		Expect(targets[0].GetTTL()).To(Equal(int64(120)))
		Expect(targets[1].GetRecordType()).To(Equal(dns.TypeAAAA))
		Expect(targets[1].GetRecordValue()).To(Equal(ipv6Address))
		Expect(targets[1].GetTTL()).To(Equal(int64(120)))
	})

	It("returns TXT targets for quoted strings", func() {
		status := &v1alpha1.DNSEntryStatus{
			Zone:    ptr.To("zone1"),
			Targets: []string{`"foo"`, `"bar"`},
		}
		targets, err := dnsentry.StatusToTargets(status, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(targets).To(HaveLen(2))
		Expect(targets[0].GetRecordType()).To(Equal(dns.TypeTXT))
		Expect(targets[0].GetRecordValue()).To(Equal(`foo`))
		Expect(targets[0].GetTTL()).To(Equal(int64(0)))
		Expect(targets[1].GetRecordType()).To(Equal(dns.TypeTXT))
		Expect(targets[1].GetRecordValue()).To(Equal(`bar`))
		Expect(targets[1].GetTTL()).To(Equal(int64(0)))
	})

	It("returns CNAME target for non-IP, non-quoted string", func() {
		status := &v1alpha1.DNSEntryStatus{
			Zone:    ptr.To("zone1"),
			Targets: []string{"foo.example.com"},
		}
		targets, err := dnsentry.StatusToTargets(status, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(targets).To(HaveLen(1))
		Expect(targets[0].GetRecordType()).To(Equal(dns.TypeCNAME))
	})

	It("skips duplicate targets", func() {
		status := &v1alpha1.DNSEntryStatus{
			Zone:    ptr.To("zone1"),
			Targets: []string{ipv4Address, ipv4Address},
		}
		targets, err := dnsentry.StatusToTargets(status, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(targets).To(HaveLen(1))
	})
})
