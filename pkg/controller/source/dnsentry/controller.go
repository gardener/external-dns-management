// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/resources"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns/source"
)

var MainResource = resources.NewGroupKind(api.GroupName, api.DNSEntryKind)

func init() {
	source.DNSSourceController(source.NewDNSSouceTypeForCreator("dnsentry-source", MainResource, NewDNSEntrySource), nil).
		FinalizerDomain("dns.gardener.cloud").
		Cluster(cluster.DEFAULT).
		CustomResourceDefinitions(MainResource).
		ActivateExplicitly().
		MustRegister()
}
