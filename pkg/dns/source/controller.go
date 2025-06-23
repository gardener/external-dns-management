// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile/reconcilers"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/watches"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/runtime"

	"github.com/gardener/external-dns-management/pkg/apis/dns/crds"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/annotation"
	"github.com/gardener/external-dns-management/pkg/dns"
)

const (
	CONTROLLER_GROUP_DNS_SOURCES = dns.CONTROLLER_GROUP_DNS_SOURCES
	TARGET_CLUSTER               = "target"
)

const (
	DNS_ANNOTATION            = dns.DNS_ANNOTATION
	TTL_ANNOTATION            = dns.ANNOTATION_GROUP + "/ttl"
	PERIOD_ANNOTATION         = dns.ANNOTATION_GROUP + "/cname-lookup-interval"
	ROUTING_POLICY_ANNOTATION = dns.ANNOTATION_GROUP + "/routing-policy"
	CLASS_ANNOTATION          = dns.CLASS_ANNOTATION
	// RESOLVE_TARGETS_TO_ADDRS_ANNOTATION is the annotation key for source objects to set the `.spec.resolveTargetsToAddresses` in the DNSEntry.
	RESOLVE_TARGETS_TO_ADDRS_ANNOTATION = dns.ANNOTATION_GROUP + "/resolve-targets-to-addresses"
)

const (
	OPT_CLASS                      = "dns-class"
	OPT_TARGET_CLASS               = "dns-target-class"
	OPT_EXCLUDE                    = "exclude-domains"
	OPT_KEY                        = "key"
	OPT_NAMESPACE                  = "target-namespace"
	OPT_NAMEPREFIX                 = "target-name-prefix"
	OPT_TARGET_CREATOR_LABEL_NAME  = "target-creator-label-name"
	OPT_TARGET_CREATOR_LABEL_VALUE = "target-creator-label-value"
	OPT_TARGET_REALMS              = "target-realms"
)

var (
	entryGroupKind = resources.NewGroupKind(api.GroupName, api.DNSEntryKind)
)

const KEY_STATE = "source-state"

func init() {
	runtime.Must(cluster.Register(TARGET_CLUSTER, "target", "target cluster for dns requests"))

	crds.AddToRegistry(apiextensions.DefaultRegistry())
}

func DNSSourceController(source DNSSourceType, reconcilerType controller.ReconcilerType) controller.Configuration {
	gk := source.GroupKind()
	return controller.Configure(source.Name()).
		After(annotation.CONTROLLER).
		RequireLease().
		DefaultedStringOption(OPT_CLASS, dns.DEFAULT_CLASS, "identifier used to differentiate responsible controllers for entries").
		StringOption(OPT_TARGET_CLASS, "identifier used to differentiate responsible dns controllers for target entries").
		StringArrayOption(OPT_EXCLUDE, "excluded domains").
		StringOption(OPT_KEY, "selecting key for annotation").
		DefaultedStringOption(OPT_NAMESPACE, "", "target namespace for cross cluster generation").
		DefaultedStringOption(OPT_NAMEPREFIX, "", "name prefix in target namespace for cross cluster generation").
		DefaultedStringOption(OPT_TARGET_CREATOR_LABEL_NAME, "creator", "label name to store the creator for generated DNS entries").
		StringOption(OPT_TARGET_CREATOR_LABEL_VALUE, "label value for creator label").
		StringOption(OPT_TARGET_REALMS, "realm(s) to use for generated DNS entries").
		FinalizerDomain(api.GroupName).
		Reconciler(SourceReconciler(source, reconcilerType)).
		Cluster(cluster.DEFAULT). // first one used as MAIN cluster
		DefaultWorkerPool(2, 120*time.Second).
		MainResource(gk.Group, gk.Kind).
		Reconciler(reconcilerTypeFilterByKind(gk.Kind, reconcilers.SlaveReconcilerTypeByFunction(SlaveReconcilerType, SlaveAccessSpecCreatorForSource(source))), "entries").
		Cluster(TARGET_CLUSTER, cluster.DEFAULT).
		CustomResourceDefinitions(entryGroupKind).
		WorkerPool("targets", 2, 0).
		ReconcilerSelectedWatchesByGK("entries", controller.NamespaceByOptionSelection(OPT_NAMESPACE), entryGroupKind)
}

var SlaveResources = reconcilers.ClusterResources(TARGET_CLUSTER, entryGroupKind)

func MasterResourcesType(kind schema.GroupKind) reconcilers.Resources {
	return func(c controller.Interface) ([]resources.Interface, error) {
		target := c.GetMainCluster()
		res, err := target.Resources().GetByGK(kind)
		if err != nil {
			return nil, err
		}
		return []resources.Interface{res}, nil
	}
}

func SlaveAccessSpecCreatorForSource(sourceType DNSSourceType) reconcilers.SlaveAccessSpecCreator {
	return func(c controller.Interface) reconcilers.SlaveAccessSpec {
		return NewSlaveAccessSpec(c, sourceType)
	}
}

func OptionIsSet(name string) watches.WatchConstraint {
	return watches.NewFunctionWatchConstraint(
		func(wctx watches.WatchContext) bool {
			s, err := wctx.GetStringOption(name)
			return err == nil && s != ""
		},
		fmt.Sprintf("string option %s is set", name),
	)
}
