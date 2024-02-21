// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package istio

import (
	"strings"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/external-dns-management/pkg/controller/source/ingress"
	"github.com/gardener/external-dns-management/pkg/controller/source/service"

	"github.com/gardener/external-dns-management/pkg/dns/source"
)

var (
	GroupKindGateway        = resources.NewGroupKind("networking.istio.io", "Gateway")
	GroupKindVirtualService = resources.NewGroupKind("networking.istio.io", "VirtualService")
)

func init() {
	source.DNSSourceController(source.NewDNSSouceTypeForCreator("istio-gateways-dns", GroupKindGateway, NewGatewaySource), nil).
		FinalizerDomain("dns.gardener.cloud").
		DeactivateOnCreationErrorCheck(deactivateOnMissingMainResource).
		Reconciler(newTargetSourcesReconciler, "targetsources").
		Reconciler(newVirtualServicesReconciler, "virtualservices").
		Cluster(cluster.DEFAULT).
		WorkerPool("targetsources", 2, 0).
		ReconcilerWatchesByGK("targetsources", service.MainResource, ingress.MainResource).
		WorkerPool("virtualservices", 2, 0).
		ReconcilerWatchesByGK("virtualservices", GroupKindVirtualService).
		MustRegister(source.CONTROLLER_GROUP_DNS_SOURCES)
}

func deactivateOnMissingMainResource(err error) bool {
	return strings.Contains(err.Error(), "gardener/cml/resources/UNKNOWN_RESOURCE") &&
		(strings.Contains(err.Error(), GroupKindGateway.String()) || strings.Contains(err.Error(), GroupKindVirtualService.String()))
}
