package config

import (
	"fmt"
	"k8s.io/apimachinery/pkg/util/yaml"
	"os"
	"regexp"
	"strings"
	"text/template"
)

var (
	kubeconfig     string
	configFilename = "functest-config.yaml"
	namespace      = "default"
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
	fmt.Printf("NAMESPACE=%s\n", namespace)
}

type ProviderConfig struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Domain        string `json:"domain"`
	ForeignDomain string `json:"foreignDomain,omitempty"`
	SecretData    string `json:"secretData"`
	Prefix        string `json:"prefix"`
	AliasTarget   string `json:"aliasTarget,omitempty"`
	ZoneID        string `json:"zoneID"`

	Namespace           string
	TmpManifestFilename string
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

	config.Utils = CreateDefaultTestUtils()

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
	}
	return nil
}

func (p *ProviderConfig) CreateTempManifest(basePath string, manifestTemplate *template.Template) error {
	p.TmpManifestFilename = ""
	filename := fmt.Sprintf("%s/tmp-%s.yaml", basePath, p.Name)
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
