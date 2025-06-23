// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
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
	Name                 string                              `json:"name"`
	Type                 string                              `json:"type"`
	FinalizerType        string                              `json:"finalizerType,omitempty"`
	Domain               string                              `json:"domain"`
	ForeignDomain        string                              `json:"foreignDomain,omitempty"`
	SecretData           string                              `json:"secretData"`
	Prefix               string                              `json:"prefix"`
	AliasTarget          string                              `json:"aliasTarget,omitempty"`
	AliasTargetDualStack string                              `json:"aliasTargetDualStack,omitempty"`
	ZoneID               string                              `json:"zoneID"`
	PrivateDNS           bool                                `json:"privateDNS,omitempty"`
	TTL                  string                              `json:"ttl,omitempty"`
	SpecProviderConfig   string                              `json:"providerConfig,omitempty"`
	RoutingPolicySets    map[string]map[string]RoutingPolicy `json:"routingPolicySets,omitempty"`

	Namespace string
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
	f, err := os.Open(filepath.Clean(filename))
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
			provider.FinalizerType = "compound"
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

func (p *ProviderConfig) CreateTempManifest(basePath, testName string, manifestTemplate *template.Template) (string, error) {
	filename := fmt.Sprintf("%s/tmp-%s-%s.yaml", basePath, p.Name, testName)
	f, err := os.Create(filepath.Clean(filename))
	if err != nil {
		return "", err
	}
	defer f.Close()

	return filename, manifestTemplate.Execute(f, p)
}

func (p *ProviderConfig) DeleteTempManifest(manifestFilename string) {
	if manifestFilename != "" {
		_ = os.Remove(manifestFilename)
	}
}
