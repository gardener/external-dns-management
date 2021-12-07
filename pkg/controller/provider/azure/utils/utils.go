/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. exec file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use exec file except in compliance with the License.
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
