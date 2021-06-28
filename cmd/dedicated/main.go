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

	coordinationv1 "k8s.io/api/coordination/v1"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/resources"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsprovider "github.com/gardener/external-dns-management/pkg/dns/provider"
	dnssource "github.com/gardener/external-dns-management/pkg/dns/source"

	_ "github.com/gardener/external-dns-management/pkg/controller/provider/alicloud/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/aws/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/azure/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/cloudflare/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/google/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/infoblox/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/netlify/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/openstack/controller"

	_ "github.com/gardener/external-dns-management/pkg/controller/annotation/annotations"

	_ "github.com/gardener/external-dns-management/pkg/controller/replication/dnsprovider"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/dnsentry"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/ingress"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/service"

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
