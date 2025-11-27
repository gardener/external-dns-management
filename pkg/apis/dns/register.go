// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:generate sh -c "CONTROLLER_GEN=$CONTROLLER_GEN bash $CONTROLLER_MANAGER_LIB_HACK_DIR/generate-crds"
// +kubebuilder:skip

package dns

import (
	_ "embed"
)

const (
	GroupName = "dns.gardener.cloud"
)

// DNSAnnotationsCRD contains the embedded DNSAnnotations CRD definition
//
//go:embed crds/dns.gardener.cloud_dnsannotations.yaml
var DNSAnnotationsCRD string

// DNSEntriesCRD contains the embedded DNSEntries CRD definition
//
//go:embed crds/dns.gardener.cloud_dnsentries.yaml
var DNSEntriesCRD string

// DNSProvidersCRD contains the embedded DNSProviders CRD definition
//
//go:embed crds/dns.gardener.cloud_dnsproviders.yaml
var DNSProvidersCRD string
