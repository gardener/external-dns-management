/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/util/yaml"
)

var (
	kubeconfig     string
	configFilename = "functest-config.yaml"
	namespace      = "default"
	dnsServer      = ""
	dnsLookup      = true
	dedicated      = false
)

func init() {
	kubeconfig = os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		panic("KUBECONFIG not set")
	}

	value := os.Getenv("DNS_LOOKUP")
	if value != "" {
		dnsLookup = strings.ToLower(value) == "true"
	}

	value = os.Getenv("DNS_DEDICATED")
	if value != "" {
		dedicated = strings.ToLower(value) == "true"
	}

	value = os.Getenv("DNS_SERVER")
	if value != "" {
		dnsServer = value
	}

	value = os.Getenv("NAMESPACE")
	if value != "" {
		namespace = value
	}

	value = os.Getenv("FUNCTEST_CONFIG")
	if value != "" {
		configFilename = value
	}
}

func PrintConfigEnv() {
	fmt.Printf("FUNCTEST_CONFIG=%s\n", configFilename)
	fmt.Printf("KUBECONFIG=%s\n", kubeconfig)
	fmt.Printf("DNS_LOOKUP=%t\n", dnsLookup)
	fmt.Printf("DNS_SERVER=%s\n", dnsServer)
	fmt.Printf("NAMESPACE=%s\n", namespace)
}

type ProviderConfig struct {
	Name               string                              `json:"name"`
	Type               string                              `json:"type"`
	FinalizerType      string                              `json:"finalizerType,omitempty"`
	Domain             string                              `json:"domain"`
	ForeignDomain      string                              `json:"foreignDomain,omitempty"`
	SecretData         string                              `json:"secretData"`
	Prefix             string                              `json:"prefix"`
	AliasTarget        string                              `json:"aliasTarget,omitempty"`
	ZoneID             string                              `json:"zoneID"`
	PrivateDNS         bool                                `json:"privateDNS,omitempty"`
	TTL                string                              `json:"ttl,omitempty"`
	SpecProviderConfig string                              `json:"providerConfig,omitempty"`
	RoutingPolicySets  map[string]map[string]RoutingPolicy `json:"routingPolicySets,omitempty"`

	Namespace           string
	TmpManifestFilename string
}

type RoutingPolicy struct {
	Type       string            `json:"type"`
	Parameters map[string]string `json:"parameters"`
	Targets    []string          `json:"targets"`
}

type Config struct {
	Providers []*ProviderConfig `json:"providers"`

	KubeConfig string
	Namespace  string
	DNSLookup  bool
	Utils      *TestUtils
}

func InitConfig() *Config {
	cfg, err := LoadConfig(configFilename)
	if err != nil {
		panic(err)
	}
	cfg.Namespace = namespace
	cfg.DNSLookup = dnsLookup
	cfg.KubeConfig = kubeconfig
	return cfg
}

func LoadConfig(filename string) (*Config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	decoder := yaml.NewYAMLOrJSONDecoder(f, 2000)
	config := &Config{}
	err = decoder.Decode(config)
	if err != nil {
		return nil, fmt.Errorf("Parsing config file %s failed with %s", filename, err)
	}

	err = config.postProcess(namespace)
	if err != nil {
		return nil, fmt.Errorf("Post processing config file %s failed with %s", filename, err)
	}

	config.Utils = CreateDefaultTestUtils(dnsServer)

	return config, nil
}

func (c *Config) postProcess(namespace string) error {
	names := map[string]*ProviderConfig{}
	reNewline := regexp.MustCompile("([\\n\\r]+)")
	for _, provider := range c.Providers {
		if provider.Name == "" {
			return fmt.Errorf("Invalid provider configuration: missing name")
		}
		if names[provider.Name] != nil {
			return fmt.Errorf("Duplicate provider %s", provider.Name)
		}
		names[provider.Name] = provider
		provider.Namespace = namespace
		if provider.FinalizerType == "" {
			if !dedicated {
				provider.FinalizerType = "compound"
			} else {
				provider.FinalizerType = provider.Type
			}
		}
		if provider.ForeignDomain == "" {
			parts := strings.SplitN(provider.Domain, ".", 2)
			if len(parts) == 2 {
				provider.ForeignDomain = "foreign." + parts[1]
			} else {
				provider.ForeignDomain = "foreign.test"
			}
		}
		if provider.Prefix == "" {
			provider.Prefix = provider.Name + "-"
		}
		indent := "    "
		provider.SecretData = indent + reNewline.ReplaceAllString(provider.SecretData, "$1"+indent)
		if provider.TTL == "" {
			provider.TTL = "101"
		}
		if provider.SpecProviderConfig != "" {
			provider.SpecProviderConfig = indent + reNewline.ReplaceAllString(provider.SpecProviderConfig, "$1"+indent)
		}
	}
	return nil
}

func (p *ProviderConfig) TTLValue() int {
	i, err := strconv.Atoi(p.TTL)
	if err != nil {
		panic(err)
	}
	return i
}

func (p *ProviderConfig) CreateTempManifest(basePath, testName string, manifestTemplate *template.Template) error {
	p.TmpManifestFilename = ""
	filename := fmt.Sprintf("%s/tmp-%s-%s.yaml", basePath, p.Name, testName)
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	p.TmpManifestFilename = filename

	return manifestTemplate.Execute(f, p)
}

func (p *ProviderConfig) DeleteTempManifest() {
	if p.TmpManifestFilename != "" {
		_ = os.Remove(p.TmpManifestFilename)
	}
}
