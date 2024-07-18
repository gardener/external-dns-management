// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package functional

import (
	"net"
	"os"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"github.com/gardener/external-dns-management/test/functional/config"
)

var basicTemplate = `
apiVersion: v1
kind: Secret
metadata:
  name: {{.Name}}
  namespace: {{.Namespace}}
type: Opaque
data:
{{.SecretData}}
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSProvider
metadata:
  name: {{.Name}}
  namespace: {{.Namespace}}
spec:
  type: {{.Type}}
  secretRef:
    name: {{.Name}}
{{if .SpecProviderConfig}}
  providerConfig:
{{.SpecProviderConfig}}
{{end}}
  domains:
    include:
      - {{.Domain}}
    exclude:
      - rp.{{.Domain}}
`

var basicTemplateEntries = `
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{.Prefix}}a-base
  namespace: {{.Namespace}}
spec:
  dnsName: {{.Domain}}
  ttl: {{.TTL}}
  targets:
  - 11.22.33.44
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{.Prefix}}a
  namespace: {{.Namespace}}
spec:
  dnsName: {{.Prefix}}a.{{.Domain}}
  ttl: {{.TTL}}
  targets:
  - 11.11.11.11
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{.Prefix}}aaaa
  namespace: {{.Namespace}}
spec:
  dnsName: {{.Prefix}}aaaa.{{.Domain}}
  ttl: {{.TTL}}
  targets:
  - 20a0::1
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{.Prefix}}mixed
  namespace: {{.Namespace}}
spec:
  dnsName: {{.Prefix}}mixed.{{.Domain}}
  ttl: {{.TTL}}
  targets:
  - 20a0::2
  - 11.11.0.11
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{.Prefix}}txt
  namespace: {{.Namespace}}
spec:
  dnsName: {{.Prefix}}txt.{{.Domain}}
  ttl: {{.TTL}}
  text:
  - "line1"
  - "line2 bla bla"
---
{{if .AliasTarget}}
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{.Prefix}}alias
  namespace: {{.Namespace}}
spec:
  dnsName: {{.Prefix}}alias.{{.Domain}}
  ttl: {{.TTL}}
  targets:
  - {{.AliasTarget}}
---
{{end}}
{{if .AliasTargetDualStack}}
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{.Prefix}}alias-ds
  namespace: {{.Namespace}}
  annotations:
    dns.gardener.cloud/ip-stack: dual-stack
spec:
  dnsName: {{.Prefix}}alias-ds.{{.Domain}}
  ttl: {{.TTL}}
  targets:
  - {{.AliasTargetDualStack}}
---
{{end}}
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{.Prefix}}wildcard
  namespace: {{.Namespace}}
spec:
  dnsName: "*.{{.Prefix}}wildcard.{{.Domain}}"
  ttl: {{.TTL}}
  targets:
  - 44.44.44.44
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{.Prefix}}cname
  namespace: {{.Namespace}}
spec:
  dnsName: {{.Prefix}}cname.{{.Domain}}
  ttl: {{.TTL}}
  targets:
  - google-public-dns-a.google.com
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{.Prefix}}cname-multi
  namespace: {{.Namespace}}
spec:
  dnsName: {{.Prefix}}cname-multi.{{.Domain}}
  ttl: {{.TTL}}
  targets:
  - google-public-dns-a.google.com
  - google-public-dns-b.google.com
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{.Prefix}}foreign
  namespace: {{.Namespace}}
spec:
  dnsName: {{.Prefix}}foreign.{{.ForeignDomain}}
  ttl: {{.TTL}}
  targets:
  - 22.22.22.22
`

func init() {
	addProviderTests(functestBasics)
}

func functestBasics(cfg *config.Config, p *config.ProviderConfig) {
	_ = Describe("basics-"+p.Name, func() {
		It("should work with "+p.Name, func() {
			tmpl1, err := template.New("Manifest").Parse(basicTemplate)
			Ω(err).ShouldNot(HaveOccurred())
			tmpl2, err := template.New("Manifest").Parse(basicTemplateEntries)
			Ω(err).ShouldNot(HaveOccurred())

			basePath, err := os.Getwd()
			Ω(err).ShouldNot(HaveOccurred())

			manifest1, err := p.CreateTempManifest(basePath, "basics", tmpl1)
			Ω(err).ShouldNot(HaveOccurred())
			defer p.DeleteTempManifest(manifest1)

			manifest2, err := p.CreateTempManifest(basePath, "basics2", tmpl2)
			Ω(err).ShouldNot(HaveOccurred())
			defer p.DeleteTempManifest(manifest2)

			ttl := p.TTLValue()

			u := cfg.Utils

			err = u.AwaitKubectlGetCRDs("dnsproviders.dns.gardener.cloud", "dnsentries.dns.gardener.cloud")
			Ω(err).ShouldNot(HaveOccurred())

			err = u.KubectlApply(manifest1)
			Ω(err).ShouldNot(HaveOccurred())
			err = u.KubectlApply(manifest2)
			Ω(err).ShouldNot(HaveOccurred())

			err = u.AwaitDNSProviderReady(p.Name)
			Ω(err).ShouldNot(HaveOccurred())

			entryNames := []string{}
			for _, name := range []string{"a", "a-base", "aaaa", "mixed", "txt", "wildcard", "cname", "cname-multi"} {
				entryNames = append(entryNames, entryName(p, name))
			}
			if p.AliasTarget != "" {
				entryNames = append(entryNames, entryName(p, "alias"))
			}
			if p.AliasTargetDualStack != "" {
				entryNames = append(entryNames, entryName(p, "alias-ds"))
			}
			err = u.AwaitDNSEntriesReady(entryNames...)
			Ω(err).ShouldNot(HaveOccurred())

			itemMap, err := u.KubectlGetAllDNSEntries()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(itemMap).Should(MatchKeys(IgnoreExtras, Keys{
				entryName(p, "a"): MatchKeys(IgnoreExtras, Keys{
					"metadata": MatchKeys(IgnoreExtras, Keys{
						"finalizers": And(HaveLen(1), ContainElement("dns.gardener.cloud/"+p.FinalizerType)),
					}),
					"spec": MatchKeys(IgnoreExtras, Keys{
						"dnsName": Equal(p.Domain),
						"targets": And(HaveLen(1), ContainElement("11.22.33.44")),
					}),
					"status": MatchKeys(IgnoreExtras, Keys{
						"message":      Equal("dns entry active"),
						"provider":     Equal(p.Namespace + "/" + p.Name),
						"providerType": Equal(p.Type),
						"state":        Equal("Ready"),
						"targets":      And(HaveLen(1), ContainElement("11.22.33.44")),
						"ttl":          Equal(float64(ttl)),
						"zone":         Equal(p.ZoneID),
					}),
				}),
				entryName(p, "a"): MatchKeys(IgnoreExtras, Keys{
					"metadata": MatchKeys(IgnoreExtras, Keys{
						"finalizers": And(HaveLen(1), ContainElement("dns.gardener.cloud/"+p.FinalizerType)),
					}),
					"spec": MatchKeys(IgnoreExtras, Keys{
						"dnsName": Equal(dnsName(p, "a")),
						"targets": And(HaveLen(1), ContainElement("11.11.11.11")),
					}),
					"status": MatchKeys(IgnoreExtras, Keys{
						"message":      Equal("dns entry active"),
						"provider":     Equal(p.Namespace + "/" + p.Name),
						"providerType": Equal(p.Type),
						"state":        Equal("Ready"),
						"targets":      And(HaveLen(1), ContainElement("11.11.11.11")),
						"ttl":          Equal(float64(ttl)),
						"zone":         Equal(p.ZoneID),
					}),
				}),
				entryName(p, "aaaa"): MatchKeys(IgnoreExtras, Keys{
					"metadata": MatchKeys(IgnoreExtras, Keys{
						"finalizers": And(HaveLen(1), ContainElement("dns.gardener.cloud/"+p.FinalizerType)),
					}),
					"spec": MatchKeys(IgnoreExtras, Keys{
						"dnsName": Equal(dnsName(p, "aaaa")),
						"targets": And(HaveLen(1), ContainElement("20a0::1")),
					}),
					"status": MatchKeys(IgnoreExtras, Keys{
						"message":      Equal("dns entry active"),
						"provider":     Equal(p.Namespace + "/" + p.Name),
						"providerType": Equal(p.Type),
						"state":        Equal("Ready"),
						"targets":      And(HaveLen(1), ContainElement("20a0::1")),
						"ttl":          Equal(float64(ttl)),
						"zone":         Equal(p.ZoneID),
					}),
				}),
				entryName(p, "mixed"): MatchKeys(IgnoreExtras, Keys{
					"metadata": MatchKeys(IgnoreExtras, Keys{
						"finalizers": And(HaveLen(1), ContainElement("dns.gardener.cloud/"+p.FinalizerType)),
					}),
					"spec": MatchKeys(IgnoreExtras, Keys{
						"dnsName": Equal(dnsName(p, "mixed")),
						"targets": And(HaveLen(2), ContainElement("20a0::2"), ContainElement("11.11.0.11")),
					}),
					"status": MatchKeys(IgnoreExtras, Keys{
						"message":      Equal("dns entry active"),
						"provider":     Equal(p.Namespace + "/" + p.Name),
						"providerType": Equal(p.Type),
						"state":        Equal("Ready"),
						"targets":      And(HaveLen(2), ContainElement("20a0::2"), ContainElement("11.11.0.11")),
						"ttl":          Equal(float64(ttl)),
						"zone":         Equal(p.ZoneID),
					}),
				}),
				entryName(p, "txt"): MatchKeys(IgnoreExtras, Keys{
					"spec": MatchKeys(IgnoreExtras, Keys{
						"dnsName": Equal(dnsName(p, "txt")),
						"text":    And(HaveLen(2), ContainElement("line1"), ContainElement("line2 bla bla")),
					}),
					"status": MatchKeys(IgnoreExtras, Keys{
						"state":   Equal("Ready"),
						"targets": And(HaveLen(2), ContainElement("\"line1\""), ContainElement("\"line2 bla bla\"")),
					}),
				}),
				entryName(p, "wildcard"): MatchKeys(IgnoreExtras, Keys{
					"spec": MatchKeys(IgnoreExtras, Keys{
						"dnsName": Equal("*." + dnsName(p, "wildcard")),
						"targets": And(HaveLen(1), ContainElement("44.44.44.44")),
					}),
					"status": MatchKeys(IgnoreExtras, Keys{
						"state":   Equal("Ready"),
						"ttl":     Equal(float64(ttl)),
						"targets": And(HaveLen(1), ContainElement("44.44.44.44")),
					}),
				}),
				entryName(p, "cname"): MatchKeys(IgnoreExtras, Keys{
					"spec": MatchKeys(IgnoreExtras, Keys{
						"dnsName": Equal(dnsName(p, "cname")),
						"targets": And(HaveLen(1), ContainElement("google-public-dns-a.google.com")),
					}),
					"status": MatchKeys(IgnoreExtras, Keys{
						"state":   Equal("Ready"),
						"targets": And(HaveLen(1), ContainElement("google-public-dns-a.google.com")),
					}),
				}),
				entryName(p, "cname-multi"): MatchKeys(IgnoreExtras, Keys{
					"spec": MatchKeys(IgnoreExtras, Keys{
						"dnsName": Equal(dnsName(p, "cname-multi")),
						"targets": And(HaveLen(2), ContainElement("google-public-dns-a.google.com"), ContainElement("google-public-dns-b.google.com")),
					}),
					"status": MatchKeys(IgnoreExtras, Keys{
						"state":   Equal("Ready"),
						"targets": And(HaveLen(4), ContainElement("8.8.8.8"), ContainElement("8.8.4.4"), ContainElement("2001:4860:4860::8888"), ContainElement("2001:4860:4860::8844")),
					}),
				}),
			}))

			if p.AliasTarget != "" {
				By("handles AliasTarget", func() {
					Ω(itemMap).Should(MatchKeys(IgnoreExtras, Keys{
						entryName(p, "alias"): MatchKeys(IgnoreExtras, Keys{
							"metadata": MatchKeys(IgnoreExtras, Keys{
								"finalizers": And(HaveLen(1), ContainElement("dns.gardener.cloud/"+p.FinalizerType)),
							}),
							"spec": MatchKeys(IgnoreExtras, Keys{
								"dnsName": Equal(dnsName(p, "alias")),
								"targets": And(HaveLen(1), ContainElement(p.AliasTarget)),
							}),
							"status": MatchKeys(IgnoreExtras, Keys{
								"message":      Equal("dns entry active"),
								"provider":     Equal(p.Namespace + "/" + p.Name),
								"providerType": Equal(p.Type),
								"state":        Equal("Ready"),
								"targets":      And(HaveLen(1), ContainElement(p.AliasTarget)),
								"ttl":          Equal(float64(ttl)),
								"zone":         Equal(p.ZoneID),
							}),
						}),
					}))
				})
			}
			if p.AliasTargetDualStack != "" {
				By("handles AliasTargetDualStack", func() {
					Ω(itemMap).Should(MatchKeys(IgnoreExtras, Keys{
						entryName(p, "alias-ds"): MatchKeys(IgnoreExtras, Keys{
							"metadata": MatchKeys(IgnoreExtras, Keys{
								"finalizers": And(HaveLen(1), ContainElement("dns.gardener.cloud/"+p.FinalizerType)),
							}),
							"spec": MatchKeys(IgnoreExtras, Keys{
								"dnsName": Equal(dnsName(p, "alias-ds")),
								"targets": And(HaveLen(1), ContainElement(p.AliasTargetDualStack)),
							}),
							"status": MatchKeys(IgnoreExtras, Keys{
								"message":      Equal("dns entry active"),
								"provider":     Equal(p.Namespace + "/" + p.Name),
								"providerType": Equal(p.Type),
								"state":        Equal("Ready"),
								"targets":      And(HaveLen(1), ContainElement(p.AliasTargetDualStack)),
								"ttl":          Equal(float64(ttl)),
								"zone":         Equal(p.ZoneID),
							}),
						}),
					}))
				})
			}

			if cfg.DNSLookup && cfg.Utils.CanLookup(p.PrivateDNS) {
				if p.AliasTarget != "" {
					u.AwaitLookupCName(dnsName(p, "alias"), p.AliasTarget)
				}
				if p.AliasTargetDualStack != "" {
					u.AwaitLookupCName(dnsName(p, "alias-ds"), p.AliasTargetDualStack)
					ips, err := net.LookupIP(dnsName(p, "alias-ds"))
					Ω(err).ShouldNot(HaveOccurred())
					var hasIPv4, hasIPv6 bool
					for _, ip := range ips {
						if ip.To4() != nil {
							hasIPv4 = true
						} else {
							hasIPv6 = true
						}
					}
					Ω(hasIPv4).Should(BeTrue())
					Ω(hasIPv6).Should(BeTrue())
				}
				u.AwaitLookup(p.Domain, "11.22.33.44")
				u.AwaitLookup(dnsName(p, "a"), "11.11.11.11")
				// no valid IPv6 addresses
				// u.AwaitLookup(dnsName(p, "aaaa"), "20a0::1")
				// u.AwaitLookup(dnsName(p, "mixed"), "20a0::2", "11.11.0.11")
				randname := config.RandStringBytes(6)
				u.AwaitLookup(randname+"."+dnsName(p, "wildcard"), "44.44.44.44")
				u.AwaitLookupCName(dnsName(p, "cname"), "google-public-dns-a.google.com")
				u.AwaitLookup(dnsName(p, "cname-multi"), "8.8.8.8", "8.8.4.4")
				// propagation of TXT entries is sometimes slower, therefore check at last
				u.AwaitLookupTXT(dnsName(p, "txt"), "line1", "line2 bla bla")
			}

			entryForeign := entryName(p, "foreign")
			// sometimes, need to wait for 120 seconds for status change to error
			u.SetTimeoutForNextAwait(130 * time.Second)
			err = u.AwaitDNSEntriesError(entryForeign)
			Ω(err).ShouldNot(HaveOccurred())
			time.Sleep(5 * time.Second)
			itemMap, err = u.KubectlGetAllDNSEntries()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(itemMap).Should(MatchKeys(IgnoreExtras, Keys{
				entryForeign: MatchKeys(IgnoreExtras, Keys{
					"metadata": MatchKeys(IgnoreMissing|IgnoreExtras, Keys{
						"finalizers": Equal("Finalizer should not be set"),
					}),
					"spec": MatchKeys(IgnoreExtras, Keys{
						"dnsName": Equal(dnsForeignName(p, "foreign")),
					}),
					"status": MatchKeys(IgnoreExtras, Keys{
						"state":   Equal("Error"),
						"message": Or(ContainSubstring("no matching provider"), ContainSubstring("No responsible provider found")),
					}),
				}),
			}))

			err = u.KubectlDelete(manifest2)
			Ω(err).ShouldNot(HaveOccurred())

			err = u.AwaitDNSEntriesDeleted(entryNames...)
			Ω(err).ShouldNot(HaveOccurred())

			err = u.AwaitDNSEntriesDeleted(entryForeign)
			Ω(err).ShouldNot(HaveOccurred())

			err = u.KubectlDelete(manifest1)
			Ω(err).ShouldNot(HaveOccurred())

			err = u.AwaitDNSProviderDeleted(p.Name)
			Ω(err).ShouldNot(HaveOccurred())
		})
	})
}

func dnsName(p *config.ProviderConfig, name string) string {
	return p.Prefix + name + "." + p.Domain
}

func dnsForeignName(p *config.ProviderConfig, name string) string {
	return p.Prefix + name + "." + p.ForeignDomain
}

func entryName(p *config.ProviderConfig, name string) string {
	return p.Prefix + name
}
