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

package main

import (
	"fmt"
	"os"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/external-dns-management/pkg/server/remote"
	"github.com/gardener/external-dns-management/pkg/server/remote/embed"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	_ "github.com/gardener/external-dns-management/pkg/controller/annotation/annotations"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/alicloud"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/aws"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/azure"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/azure-private"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/cloudflare"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/compound/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/google"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/infoblox"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/netlify"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/openstack"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/remote"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/rfc2136"
	_ "github.com/gardener/external-dns-management/pkg/controller/remoteaccesscertificates"
	_ "github.com/gardener/external-dns-management/pkg/controller/replication/dnsprovider"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/dnsentry"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/gateways/istio"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/ingress"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/service"
	dnsprovider "github.com/gardener/external-dns-management/pkg/dns/provider"
	dnssource "github.com/gardener/external-dns-management/pkg/dns/source"
	_ "github.com/gardener/external-dns-management/pkg/server/pprof"

	_ "go.uber.org/automaxprocs"
	coordinationv1 "k8s.io/api/coordination/v1"
	networkingv1 "k8s.io/api/networking/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var Version string

func init() {
	cluster.Configure(
		dnsprovider.PROVIDER_CLUSTER,
		"providers",
		"cluster to look for provider objects",
	).Fallback(dnssource.TARGET_CLUSTER).MustRegister()

	mappings.ForControllerGroup(dnsprovider.CONTROLLER_GROUP_DNS_CONTROLLERS).
		Map(controller.CLUSTER_MAIN, dnssource.TARGET_CLUSTER).MustRegister()

	resources.Register(v1alpha1.SchemeBuilder)
	resources.Register(coordinationv1.SchemeBuilder)
	resources.Register(networkingv1.SchemeBuilder)

	embed.RegisterCreateServerFunc(remote.CreateServer)
}

func migrateExtensionsIngress(c controllermanager.Configuration) controllermanager.Configuration {
	return c.GlobalGroupKindMigrations(resources.NewGroupKind("extensions", "Ingress"),
		resources.NewGroupKind("networking.k8s.io", "Ingress"))
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Println(Version)
		os.Exit(0)
	}
	controllermanager.Start("dns-controller-manager", "dns controller manager", "nothing", migrateExtensionsIngress)
}
