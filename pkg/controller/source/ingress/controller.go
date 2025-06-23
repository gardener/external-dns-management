// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ingress

import (
	"github.com/gardener/controller-manager-library/pkg/resources"

	"github.com/gardener/external-dns-management/pkg/dns/source"
)

var MainResource = resources.NewGroupKind("networking.k8s.io", "Ingress")

func init() {
	source.DNSSourceController(source.NewDNSSouceTypeForCreator("ingress-dns", MainResource, NewIngressSource), nil).
		FinalizerDomain("dns.gardener.cloud").
		MustRegister(source.CONTROLLER_GROUP_DNS_SOURCES)
}
