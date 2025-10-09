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

	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/azure/constants"
)

const (
	defaultMaxRetries    = 3
	defaultMaxRetryDelay = math.MaxInt64
	defaultRetryDelay    = 5 * time.Second
)

var re = regexp.MustCompile("/resourceGroups/([^/]+)/")

// ExtractResourceGroup extracts the resource group from a given resource ID.
func ExtractResourceGroup(id string) (string, error) {
	submatches := re.FindStringSubmatch(id)
	if len(submatches) != 2 {
		return "", fmt.Errorf("unexpected DNS zone id %q", id)
	}
	return submatches[1], nil
}

// DropZoneName shortens DNS entry name from record name + zone to record name only: e.g www2.test6227.ml to www2
func DropZoneName(dnsName, zoneName string) (string, bool) {
	trimmed := strings.TrimSuffix(dnsName, fmt.Sprintf(".%s", zoneName))
	return trimmed, trimmed != dnsName
}

// MakeZoneID creates a zone ID from the resource group and zone name
func MakeZoneID(resourceGroup, zoneName string) (string, error) {
	if strings.Contains(resourceGroup, "/") {
		return "", fmt.Errorf("resourceGroup must not contain '/': %s", resourceGroup)
	}
	if strings.Contains(zoneName, "/") {
		return "", fmt.Errorf("zoneName must not contain '/': %s", zoneName)
	}
	return resourceGroup + "/" + zoneName, nil
}

// SplitZoneID returns resource group and zone name for a zoneID
func SplitZoneID(zoneID string) (string, string) {
	parts := strings.Split(zoneID, "/")
	if len(parts) != 2 {
		return "", zoneID
	}
	return parts[0], parts[1]
}

// GetSubscriptionIdAndCredentials retrieves the subscription ID, client ID, client secret, and tenant ID from the config and uses them to create a TokenCredential.
// See also: https://docs.microsoft.com/en-us/go/azure/azure-sdk-go-authorization
func GetSubscriptionIdAndCredentials(c *provider.DNSHandlerConfig) (subscriptionID string, tokenCredential azcore.TokenCredential, err error) {
	subscriptionID, err = c.GetRequiredProperty(constants.PropertySubscriptionID, constants.PropertySubscriptionIDAlias)
	if err != nil {
		return
	}
	clientID, err := c.GetRequiredProperty(constants.PropertyClientID, constants.PropertyClientIDAlias)
	if err != nil {
		return
	}
	clientSecret, err := c.GetRequiredProperty(constants.PropertyClientSecret, constants.PropertyClientSecretAlias)
	if err != nil {
		return
	}
	tenantID, err := c.GetRequiredProperty(constants.PropertyTenantID, constants.PropertyTenantIDAlias)
	if err != nil {
		return
	}
	tokenCredential, err = azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		err = perrs.WrapAsHandlerError(err, "creating Azure authorizer with client credentials failed")
	}
	return
}

// GetDefaultAzureClientOpts returns default Azure client options with retry and transport settings.
func GetDefaultAzureClientOpts(c *provider.DNSHandlerConfig) (*arm.ClientOptions, error) {
	cloudConf, err := getAzureCloudConfiguration(c)
	if err != nil {
		return nil, err
	}
	return &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Retry: policy.RetryOptions{
				RetryDelay:    defaultRetryDelay,
				MaxRetryDelay: defaultMaxRetryDelay,
				MaxRetries:    defaultMaxRetries,
				StatusCodes:   getRetriableStatusCode(),
			},
			Transport: &http.Client{
				Transport: getTransport(),
			},
			Cloud: *cloudConf,
		},
	}, nil
}

func getAzureCloudConfiguration(c *provider.DNSHandlerConfig) (*cloud.Configuration, error) {
	switch v := c.GetProperty(constants.PropertyCloud); v {
	case "", "AzurePublic":
		return &cloud.AzurePublic, nil
	case "AzureChina":
		return &cloud.AzureChina, nil
	case "AzureGovernment":
		return &cloud.AzureGovernment, nil
	default:
		return nil, fmt.Errorf("unknown cloud configuration name '%s'", v)
	}
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
