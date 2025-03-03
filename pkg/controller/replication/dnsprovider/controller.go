// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsprovider

import (
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile/reconcilers"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"
	"k8s.io/apimachinery/pkg/util/runtime"

	"github.com/gardener/external-dns-management/pkg/apis/dns/crds"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"
)

var gkDNSProvider = resources.NewGroupKind(api.GroupName, api.DNSProviderKind)

func init() {
	runtime.Must(cluster.Register(source.TARGET_CLUSTER, "target", "target cluster for dns requests"))

	crds.AddToRegistry(apiextensions.DefaultRegistry())

	controller.Configure("dnsprovider-replication").
		RequireLease().
		DefaultedStringOption(source.OPT_CLASS, dns.DEFAULT_CLASS, "identifier used to differentiate responsible controllers for providers").
		StringOption(source.OPT_TARGET_CLASS, "identifier used to differentiate responsible dns controllers for target providers").
		DefaultedStringOption(source.OPT_NAMESPACE, "", "target namespace for cross cluster generation").
		DefaultedStringOption(source.OPT_NAMEPREFIX, "source-", "name prefix in target namespace for cross cluster replication").
		DefaultedStringOption(source.OPT_TARGET_CREATOR_LABEL_NAME, "creator", "label name to store the creator for replicated DNS providers").
		StringOption(source.OPT_TARGET_CREATOR_LABEL_VALUE, "label value for creator label").
		StringOption(source.OPT_TARGET_REALMS, "realm(s) to use for replicated DNS provider").
		FinalizerDomain(api.GroupName).
		Reconciler(DNSProviderReplicationReconciler).
		Cluster(cluster.DEFAULT).             // first one used as MAIN cluster
		DefaultWorkerPool(2, 10*time.Minute). // period reconcile as provider secrets are not watched
		MainResource(gkDNSProvider.Group, gkDNSProvider.Kind).
		CustomResourceDefinitions(gkDNSProvider).
		Reconciler(reconcilers.SlaveReconcilerTypeByFunction(SlaveReconcilerType, SlaveAccessSpecCreator()), "providers").
		Cluster(source.TARGET_CLUSTER, cluster.DEFAULT).
		CustomResourceDefinitions(gkDNSProvider).
		WorkerPool("targets", 2, 0).
		ReconcilerSelectedWatchesByGK("providers", controller.NamespaceByOptionSelection(source.OPT_NAMESPACE), gkDNSProvider).
		FinalizerDomain("dns.gardener.cloud").
		ActivateExplicitly().
		MustRegister(dns.CONTROLLER_GROUP_REPLICATION)
}

func SlaveAccessSpecCreator() reconcilers.SlaveAccessSpecCreator {
	return func(c controller.Interface) reconcilers.SlaveAccessSpec {
		return NewSlaveAccessSpec(c)
	}
}
