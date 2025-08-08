// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/controller/provider/infoblox/config"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

// ProviderType is the type code for the Infoblox DNS provider.
const ProviderType = "infoblox-dns"

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

// NewAdapter creates a new DNSHandlerAdapter for the Infoblox DNS provider.
func NewAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()
	checks.Add(provider.RequiredProperty("USERNAME", "username").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(64)))
	checks.Add(provider.RequiredProperty("PASSWORD", "password").
		Validators(provider.NoTrailingWhitespaceValidator, provider.MaxLengthValidator(64)).
		HideValue())
	checks.Add(provider.RequiredProperty("VERSION", "version").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(64)))
	checks.Add(provider.OptionalProperty("VIEW", "view").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(64)))
	checks.Add(provider.RequiredProperty("HOST", "host").
		Validators(provider.NoTrailingWhitespaceValidator, provider.AlphaNumericPunctuationValidator, provider.MaxLengthValidator(256)))
	checks.Add(provider.RequiredProperty("PORT", "port").
		Validators(provider.NoTrailingWhitespaceValidator, provider.IntValidator(1, 65535)))
	checks.Add(provider.OptionalProperty("HTTP_POOL_CONNECTIONS", "http_pool_connections", "httpPoolConnections").
		Validators(provider.NoTrailingWhitespaceValidator, provider.IntValidator(1, 100)))
	checks.Add(provider.OptionalProperty("HTTP_REQUEST_TIMEOUT", "http_request_timeout", "httpRequestTimeout").
		Validators(provider.NoTrailingWhitespaceValidator, provider.IntValidator(0, 1000)))
	checks.Add(provider.OptionalProperty("PROXY_URL", "proxy_url", "proxyUrl").
		Validators(provider.NoTrailingWhitespaceValidator, provider.URLValidator("http", "https"), provider.MaxLengthValidator(6)))
	checks.Add(provider.OptionalProperty("CA_CERT", "ca_cert", "caCert").
		Validators(provider.CACertValidator).
		HideValue())
	checks.Add(provider.OptionalProperty("SSL_VERIFY", "ssl_verify", "sslVerify").
		Validators(provider.BoolValidator))
	return &adapter{checks: checks}
}

func (a *adapter) ProviderType() string {
	return ProviderType
}

func (a *adapter) ValidateCredentialsAndProviderConfig(properties utils.Properties, rawExtension *runtime.RawExtension) error {
	if rawExtension != nil && len(rawExtension.Raw) > 0 {
		infobloxConfig := &config.InfobloxConfig{}
		if rawExtension != nil {
			err := json.Unmarshal(rawExtension.Raw, infobloxConfig)
			if err != nil {
				return fmt.Errorf("unmarshal infoblox providerConfig failed with: %s", err)
			}
		}
		if infobloxConfig.CaCert != nil && *infobloxConfig.CaCert != "" {
			a.addPropertyIfNotExists(properties, "CA_CERT", *infobloxConfig.CaCert)
		}
		if infobloxConfig.Host != nil && *infobloxConfig.Host != "" {
			a.addPropertyIfNotExists(properties, "HOST", *infobloxConfig.Host)
		}
		if infobloxConfig.PoolConnections != nil && *infobloxConfig.PoolConnections != 0 {
			a.addPropertyIfNotExists(properties, "HTTP_POOL_CONNECTIONS", strconv.FormatInt(int64(*infobloxConfig.PoolConnections), 10))
		}
		if infobloxConfig.Port != nil && *infobloxConfig.Port != 0 {
			a.addPropertyIfNotExists(properties, "PORT", strconv.FormatInt(int64(*infobloxConfig.Port), 10))
		}
		if infobloxConfig.ProxyURL != nil && *infobloxConfig.ProxyURL != "" {
			a.addPropertyIfNotExists(properties, "PROXY_URL", *infobloxConfig.ProxyURL)
		}
		if infobloxConfig.RequestTimeout != nil {
			a.addPropertyIfNotExists(properties, "HTTP_REQUEST_TIMEOUT", strconv.FormatInt(int64(*infobloxConfig.RequestTimeout), 10))
		}
		if infobloxConfig.SSLVerify != nil {
			a.addPropertyIfNotExists(properties, "SSL_VERIFY", strconv.FormatBool(*infobloxConfig.SSLVerify))
		}
		if infobloxConfig.Version != nil && *infobloxConfig.Version != "" {
			a.addPropertyIfNotExists(properties, "VERSION", *infobloxConfig.Version)
		}
		if infobloxConfig.View != nil && *infobloxConfig.View != "" {
			a.addPropertyIfNotExists(properties, "VIEW", *infobloxConfig.View)
		}
	}
	return a.checks.ValidateProperties(a.ProviderType(), properties)
}

func (a *adapter) addPropertyIfNotExists(properties utils.Properties, key string, value string) {
	if a.checks.HasPropertyNameOrAlias(properties, key) {
		return // Property already exists, do not overwrite
	}
	properties[key] = value
}
