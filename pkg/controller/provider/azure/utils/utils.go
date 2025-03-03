// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
)

var re = regexp.MustCompile("/resourceGroups/([^/]+)/")

const (
	// DefaultMaxRetries is the default value for max retries on retryable operations.
	DefaultMaxRetries = 3
	// DefaultMaxRetryDelay is the default maximum value for delay on retryable operations.
	DefaultMaxRetryDelay = math.MaxInt64
	// DefaultRetryDelay is the default value for the initial delay on retry for retryable operations.
	DefaultRetryDelay = 5 * time.Second
)

func ExtractResourceGroup(id string) (string, error) {
	submatches := re.FindStringSubmatch(id)
	if len(submatches) != 2 {
		return "", fmt.Errorf("unexpected DNS Zone ID: '%s'", id)
	}
	return submatches[1], nil
}

// DropZoneName shortens DnsEntry-dnsName from record name + .DNSZone to record name only: e.g www2.test6227.ml to www2
func DropZoneName(dnsName, zoneName string) (string, bool) {
	end := len(dnsName) - len(zoneName) - 1
	if end <= 0 || !strings.HasSuffix(dnsName, zoneName) || dnsName[end] != '.' {
		return dnsName, false
	}
	return dnsName[:end], true
}

// MakeZoneID creates zone ID from resource group and name
func MakeZoneID(resourceGroup, zoneName string) string {
	return resourceGroup + "/" + zoneName
}

// SplitZoneID returns resource group and name for a zoneid
func SplitZoneID(zoneid string) (string, string) {
	parts := strings.Split(zoneid, "/")
	if len(parts) != 2 {
		return "", zoneid
	}
	return parts[0], parts[1]
}

// GetSubscriptionIDAndCredentials extracts credentials from config
func GetSubscriptionIDAndCredentials(c *provider.DNSHandlerConfig) (subscriptionID string, tc azcore.TokenCredential, err error) {
	subscriptionID, err = c.GetRequiredProperty("AZURE_SUBSCRIPTION_ID", "subscriptionID")
	if err != nil {
		return
	}

	// see https://docs.microsoft.com/en-us/go/azure/azure-sdk-go-authorization
	clientID, err := c.GetRequiredProperty("AZURE_CLIENT_ID", "clientID")
	if err != nil {
		return
	}
	clientSecret, err := c.GetRequiredProperty("AZURE_CLIENT_SECRET", "clientSecret")
	if err != nil {
		return
	}
	tenantID, err := c.GetRequiredProperty("AZURE_TENANT_ID", "tenantID")
	if err != nil {
		return
	}

	tc, err = azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		err = perrs.WrapAsHandlerError(err, "Creating Azure authorizer with client credentials failed")
	}
	return
}

func GetDefaultAzureClientOpts(c *provider.DNSHandlerConfig) (*arm.ClientOptions, error) {
	var cloudConf cloud.Configuration
	switch v := c.GetProperty("AZURE_CLOUD"); v {
	case "", "AzurePublic":
		cloudConf = cloud.AzurePublic
	case "AzureChina":
		cloudConf = cloud.AzureChina
	case "AzureGovernment":
		cloudConf = cloud.AzureGovernment
	default:
		return nil, fmt.Errorf("unknown cloud configuration name '%s'", v)
	}

	return &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Retry: policy.RetryOptions{
				RetryDelay:    DefaultRetryDelay,
				MaxRetryDelay: DefaultMaxRetryDelay,
				MaxRetries:    DefaultMaxRetries,
				StatusCodes:   getRetriableStatusCode(),
			},
			Transport: &http.Client{
				Transport: getTransport(),
			},
			Cloud: cloudConf,
		},
	}, nil
}

func getTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        100,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
}

func getRetriableStatusCode() []int {
	return []int{
		http.StatusRequestTimeout,      // 408
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}
}
