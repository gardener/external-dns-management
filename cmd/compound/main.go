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
	istionetworkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	istionetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	coordinationv1 "k8s.io/api/coordination/v1"
	networkingv1 "k8s.io/api/networking/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapisv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

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
	_ "github.com/gardener/external-dns-management/pkg/controller/source/gateways/crdwatch"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/gateways/gatewayapi"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/gateways/istio"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/ingress"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/service"
	dnsprovider "github.com/gardener/external-dns-management/pkg/dns/provider"
	dnssource "github.com/gardener/external-dns-management/pkg/dns/source"
	_ "github.com/gardener/external-dns-management/pkg/server/pprof"
	"github.com/gardener/external-dns-management/pkg/server/remote"
	"github.com/gardener/external-dns-management/pkg/server/remote/embed"
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
	resources.Register(istionetworkingv1alpha3.SchemeBuilder)
	resources.Register(istionetworkingv1beta1.SchemeBuilder)
	resources.Register(istionetworkingv1.SchemeBuilder)
	resources.Register(gatewayapisv1alpha2.SchemeBuilder)
	resources.Register(gatewayapisv1beta1.SchemeBuilder)
	resources.Register(gatewayapisv1.SchemeBuilder)

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
