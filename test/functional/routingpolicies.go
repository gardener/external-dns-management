// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package functional

import (
	"fmt"
	"os"
	"text/template"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"github.com/gardener/external-dns-management/test/functional/config"
)

var routingPolicyTemplate = `
apiVersion: v1
kind: Secret
metadata:
  name: {{.Name}}-routingpolicies
  namespace: {{.Namespace}}
type: Opaque
data:
{{.SecretData}}
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSProvider
metadata:
  name: {{.Name}}-routingpolicies
  namespace: {{.Namespace}}
spec:
  type: {{.Type}}
  secretRef:
    name: {{.Name}}-routingpolicies
{{if .SpecProviderConfig}}
  providerConfig:
{{.SpecProviderConfig}}
{{end}}
  domains:
    include:
      - rp.{{.Domain}}
`

var routingPolicyTemplateEntries = `
{{ range $k, $v := .RoutingPolicySets }}
{{ range $id, $policy := $v }}
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: {{$.Prefix}}{{$k}}-{{$id}}
  namespace: {{$.Namespace}}
spec:
  dnsName: {{$.Prefix}}{{$k}}.rp.{{$.Domain}}
  ttl: {{$.TTL}}
  targets:
{{ range $j, $t := $policy.Targets }}
  - {{$t}}
{{ end }}
  routingPolicy:
    type: {{$policy.Type}}  
    setIdentifier: '{{$id}}'
    parameters:
{{ range $pk, $pv := $policy.Parameters }}
      {{$pk}}: '{{$pv}}'
{{ end }}
{{ end }}
{{ end }}
`

func init() {
	addProviderTests(functestRoutingPolicies)
}

func functestRoutingPolicies(cfg *config.Config, p *config.ProviderConfig) {
	_ = Describe("routingpolicies-"+p.Name, func() {
		It("should work with "+p.Name, func() {
			if len(p.RoutingPolicySets) == 0 {
				Skip("no routing policy sets defined")
			}
			tmpl1, err := template.New("Manifest").Parse(routingPolicyTemplate)
			Ω(err).ShouldNot(HaveOccurred())
			tmpl2, err := template.New("Manifest").Parse(routingPolicyTemplateEntries)
			Ω(err).ShouldNot(HaveOccurred())

			basePath, err := os.Getwd()
			Ω(err).ShouldNot(HaveOccurred())

			manifest1, err := p.CreateTempManifest(basePath, "routingpolicies", tmpl1)
			Ω(err).ShouldNot(HaveOccurred())
			defer p.DeleteTempManifest(manifest1)
			manifest2, err := p.CreateTempManifest(basePath, "routingpolicies2", tmpl2)
			Ω(err).ShouldNot(HaveOccurred())
			defer p.DeleteTempManifest(manifest2)

			u := cfg.Utils

			err = u.AwaitKubectlGetCRDs("dnsproviders.dns.gardener.cloud", "dnsentries.dns.gardener.cloud")
			Ω(err).ShouldNot(HaveOccurred())

			err = u.KubectlApply(manifest1)
			Ω(err).ShouldNot(HaveOccurred())
			err = u.KubectlApply(manifest2)
			Ω(err).ShouldNot(HaveOccurred())

			providerName := p.Name + "-routingpolicies"
			err = u.AwaitDNSProviderReady(providerName)
			Ω(err).ShouldNot(HaveOccurred())

			entryNames := []string{}
			for k, v := range p.RoutingPolicySets {
				for id := range v {
					name := entryName(p, fmt.Sprintf("%s-%s", k, id))
					entryNames = append(entryNames, name)
				}
			}

			err = u.AwaitDNSEntriesReady(entryNames...)
			Ω(err).ShouldNot(HaveOccurred())

			itemMap, err := u.KubectlGetAllDNSEntries()
			Ω(err).ShouldNot(HaveOccurred())

			for k, v := range p.RoutingPolicySets {
				for id, policy := range v {
					params := map[string]any{}
					for k, v := range policy.Parameters {
						params[k] = v
					}
					name := entryName(p, fmt.Sprintf("%s-%s", k, id))
					Ω(itemMap).Should(MatchKeys(IgnoreExtras, Keys{
						name: MatchKeys(IgnoreExtras, Keys{
							"metadata": MatchKeys(IgnoreExtras, Keys{
								"finalizers": And(HaveLen(1), ContainElement("dns.gardener.cloud/"+p.FinalizerType)),
							}),
							"spec": MatchKeys(IgnoreExtras, Keys{
								"dnsName": Equal(dnsNameRp(p, k)),
								"targets": And(HaveLen(len(policy.Targets)), ContainElements(policy.Targets)),
							}),
							"status": MatchKeys(IgnoreExtras, Keys{
								"message":      Equal("dns entry active"),
								"provider":     Equal(p.Namespace + "/" + providerName),
								"providerType": Equal(p.Type),
								"state":        Equal("Ready"),
								"targets":      And(HaveLen(len(policy.Targets)), ContainElements(policy.Targets)),
								"zone":         Equal(p.ZoneID),
								"routingPolicy": MatchAllKeys(Keys{
									"type":          Equal(policy.Type),
									"setIdentifier": Equal(id),
									"parameters":    Equal(params),
								}),
							}),
						}),
					}))
				}
			}

			err = u.KubectlDelete(manifest2)
			Ω(err).ShouldNot(HaveOccurred())

			err = u.AwaitDNSEntriesDeleted(entryNames...)
			Ω(err).ShouldNot(HaveOccurred())

			err = u.KubectlDelete(manifest1)
			Ω(err).ShouldNot(HaveOccurred())

			err = u.AwaitDNSProviderDeleted(providerName)
			Ω(err).ShouldNot(HaveOccurred())
		})
	})
}

func dnsNameRp(p *config.ProviderConfig, name string) string {
	return p.Prefix + name + ".rp." + p.Domain
}
