// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gatewayapi

import (
	"strings"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/resources"

	"github.com/gardener/external-dns-management/pkg/dns/source"
)

const Group = "gateway.networking.k8s.io"

var (
	GroupKindGateway   = resources.NewGroupKind(Group, "Gateway")
	GroupKindHTTPRoute = resources.NewGroupKind(Group, "HTTPRoute")
	Deactivated        bool
)

func init() {
	source.DNSSourceController(source.NewDNSSouceTypeForCreator("k8s-gateways-dns", GroupKindGateway, NewGatewaySource), nil).
		FinalizerDomain("dns.gardener.cloud").
		DeactivateOnCreationErrorCheck(deactivateOnMissingMainResource).
		Reconciler(HTTPRoutesReconciler, "httproutes").
		Cluster(cluster.DEFAULT).
		WorkerPool("httproutes", 2, 0).
		ReconcilerWatchesByGK("httproutes", GroupKindHTTPRoute).
		MustRegister(source.CONTROLLER_GROUP_DNS_SOURCES)
}

func deactivateOnMissingMainResource(err error) bool {
	Deactivated = strings.Contains(err.Error(), "gardener/cml/resources/UNKNOWN_RESOURCE") &&
		(strings.Contains(err.Error(), GroupKindGateway.String()) || strings.Contains(err.Error(), GroupKindHTTPRoute.String()))
	return Deactivated
}
