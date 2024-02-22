// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	perrs "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
)

var re = regexp.MustCompile("/resourceGroups/([^/]+)/")

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

// GetSubscriptionIDAndAuthorizer extracts credentials from config
func GetSubscriptionIDAndAuthorizer(c *provider.DNSHandlerConfig) (subscriptionID string, authorizer autorest.Authorizer, err error) {
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

	authorizer, err = auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID).Authorizer()
	if err != nil {
		err = perrs.WrapAsHandlerError(err, "Creating Azure authorizer with client credentials failed")
		return
	}
	return
}
