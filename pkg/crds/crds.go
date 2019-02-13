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

package crds

import (
	"github.com/gardener/controller-manager-library/pkg/clientsets/apiextensions"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var DNSOwnerCRD = apiextensions.CreateCRDObject(api.GroupName, api.Version, api.DNSOwnerKind, api.DNSOwnerPlural, "dnso", false,
	v1beta1.CustomResourceColumnDefinition{
		Name:        "OWNERID",
		Description: "Owner Id",
		Type:        "string",
		JSONPath:    ".spec.ownerId",
	})

var DNSProviderCRD = apiextensions.CreateCRDObject(api.GroupName, api.Version, api.DNSProviderKind, api.DNSProviderPlural, "dnspr", true,
	v1beta1.CustomResourceColumnDefinition{
		Name:        "TYPE",
		Description: "Provider type",
		Type:        "string",
		JSONPath:    ".status.providerType",
	},
	v1beta1.CustomResourceColumnDefinition{
		Name:        "STATUS",
		Description: "Status of DNS provider",
		Type:        "string",
		JSONPath:    ".status.state",
	})

var DNSEntryCRD = apiextensions.CreateCRDObjectWithStatus(api.GroupName, api.Version, api.DNSEntryKind, api.DNSEntryPlural, "dnse", true,
	v1beta1.CustomResourceColumnDefinition{
		Name:        "DNS",
		Description: "DNS ObjectName",
		Type:        "string",
		JSONPath:    ".spec.dnsName",
	},
	v1beta1.CustomResourceColumnDefinition{
		Name:        "STATUS",
		Description: "Status of DNS entry in cloud provider",
		Type:        "string",
		JSONPath:    ".status.state",
	})
