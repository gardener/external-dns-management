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
	return strings.Contains(err.Error(), "gardener/cml/resources/UNKNOWN_RESOURCE") &&
		(strings.Contains(err.Error(), GroupKindGateway.String()) || strings.Contains(err.Error(), GroupKindHTTPRoute.String()))
}
