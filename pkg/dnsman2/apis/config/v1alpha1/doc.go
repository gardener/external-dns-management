// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// +k8s:deepcopy-gen=package
// +k8s:conversion-gen=github.com/gardener/external-dns-management/pkg/dnsman2/apis/config
// +k8s:openapi-gen=true
// +k8s:defaulter-gen=TypeMeta

//go:generate gen-crd-api-reference-docs -api-dir github.com/gardener/external-dns-management/pkg/dnsman2/apis/config/v1alpha1 -config ../../../../../hack/api-reference/config.json -template-dir ../../../../../hack/api-reference/template -out-file ../../../../../docs/dnsman2/api-reference/dnsmanagerconfiguration.md

// +groupName=config.dns.gardener.cloud
package v1alpha1 // import "github.com/gardener/external-dns-management/pkg/dnsman2/apis/config/v1alpha1"
