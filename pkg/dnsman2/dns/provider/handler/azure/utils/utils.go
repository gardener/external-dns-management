// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
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
	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"

	workloadidentityazure "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/azure"
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

// GetSubscriptionIdAndCredentials extracts credentials from config
func GetSubscriptionIdAndCredentials(c *provider.DNSHandlerConfig) (string, azcore.TokenCredential, error) {
	if c.GetProperty(securityv1alpha1constants.LabelWorkloadIdentityProvider) == "azure" {
		token, err := c.GetRequiredProperty(securityv1alpha1constants.DataKeyToken)
		if err != nil {
			return "", nil, err
		}
		configData, err := c.GetRequiredProperty(securityv1alpha1constants.DataKeyConfig)
		if err != nil {
			return "", nil, err
		}
		wlConfig, err := workloadidentityazure.GetWorkloadIdentityConfig([]byte(configData))
		if err != nil {
			return "", nil, fmt.Errorf("invalid workload identity config: %w", err)
		}
		tokenRetriever := func(_ context.Context) (string, error) {
			return token, nil
		}
		cred, err := azidentity.NewClientAssertionCredential(wlConfig.TenantID, wlConfig.ClientID, tokenRetriever, nil)
		if err != nil {
			return "", nil, fmt.Errorf("creating Azure authorizer with workload identity failed: %w", err)
		}
		return wlConfig.SubscriptionID, cred, nil
	}

	subscriptionID, err := c.GetRequiredProperty(constants.PropertySubscriptionID, constants.PropertySubscriptionIDAlias)
	if err != nil {
		return "", nil, err
	}
	clientID, err := c.GetRequiredProperty(constants.PropertyClientID, constants.PropertyClientIDAlias)
	if err != nil {
		return "", nil, err
	}
	clientSecret, err := c.GetRequiredProperty(constants.PropertyClientSecret, constants.PropertyClientSecretAlias)
	if err != nil {
		return "", nil, err
	}
	tenantID, err := c.GetRequiredProperty(constants.PropertyTenantID, constants.PropertyTenantIDAlias)
	if err != nil {
		return "", nil, err
	}
	tokenCredential, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		err = perrs.WrapAsHandlerError(err, "creating Azure authorizer with client credentials failed")
		return "", nil, err
	}
	return subscriptionID, tokenCredential, nil
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

// StableError converts an Azure SDK error into a stable error message without correlation ID and timestamps
// to avoid endless status update/reconcile loop.
func StableError(err error) error {
	msg := err.Error()
	if index := strings.Index(msg, "\n------"); index != -1 {
		msg = msg[:index]
		return fmt.Errorf("%s - details see log", msg)
	}
	return err
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
