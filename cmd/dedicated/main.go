// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/resources"
	_ "go.uber.org/automaxprocs"
	coordinationv1 "k8s.io/api/coordination/v1"
	networkingv1 "k8s.io/api/networking/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	_ "github.com/gardener/external-dns-management/pkg/controller/annotation/annotations"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/alicloud/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/aws/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/azure-private/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/azure/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/cloudflare/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/google/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/infoblox/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/netlify/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/openstack/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/remote/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/provider/rfc2136/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/remoteaccesscertificates"
	_ "github.com/gardener/external-dns-management/pkg/controller/replication/dnsprovider"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/dnsentry"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/gateways/crdwatch"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/gateways/gatewayapi"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/gateways/istio"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/ingress"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/service"
	dnsprovider "github.com/gardener/external-dns-management/pkg/dns/provider"
	dnssource "github.com/gardener/external-dns-management/pkg/dns/source"
	_ "github.com/gardener/external-dns-management/pkg/server/pprof"
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
