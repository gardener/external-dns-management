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

package source

import (
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/watches"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"
	"k8s.io/apimachinery/pkg/util/runtime"

	"github.com/gardener/external-dns-management/pkg/apis/dns/crds"
	"github.com/gardener/external-dns-management/pkg/controller/annotation"
	"github.com/gardener/external-dns-management/pkg/dns"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile/reconcilers"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"k8s.io/apimachinery/pkg/runtime/schema"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
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
	OWNER_ID_ANNOTATION       = dns.ANNOTATION_GROUP + "/owner-id"
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
	OPT_TARGET_OWNER_ID            = "target-owner-id"
	OPT_TARGET_OWNER_OBJECT        = "target-owner-object"
	OPT_TARGET_SET_IGNORE_OWNERS   = "target-set-ignore-owners"
	OPT_TARGET_REALMS              = "target-realms"
)

var (
	entryGroupKind = resources.NewGroupKind(api.GroupName, api.DNSEntryKind)
	ownerGroupKind = resources.NewGroupKind(api.GroupName, api.DNSOwnerKind)
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
		StringOption(OPT_TARGET_OWNER_ID, "owner id to use for generated DNS entries").
		StringOption(OPT_TARGET_OWNER_OBJECT, "owner object to use for generated DNS entries").
		BoolOption(OPT_TARGET_SET_IGNORE_OWNERS, "mark generated DNS entries to omit owner based access control").
		StringOption(OPT_TARGET_REALMS, "realm(s) to use for generated DNS entries").
		FinalizerDomain(api.GroupName).
		Reconciler(SourceReconciler(source, reconcilerType)).
		Cluster(cluster.DEFAULT). // first one used as MAIN cluster
		DefaultWorkerPool(2, 120*time.Second).
		MainResource(gk.Group, gk.Kind).
		Reconciler(reconcilers.SlaveReconcilerTypeByFunction(SlaveReconcilerType, SlaveAccessSpecCreatorForSource(source)), "entries").
		Reconciler(OwnerReconciler, "owner").
		Cluster(TARGET_CLUSTER, cluster.DEFAULT).
		CustomResourceDefinitions(entryGroupKind).
		WorkerPool("targets", 2, 0).
		ReconcilerSelectedWatchesByGK("entries", controller.NamespaceByOptionSelection(OPT_NAMESPACE), entryGroupKind).
		FlavoredReconcilerWatch("owner",
			watches.Conditional(
				OptionIsSet(OPT_TARGET_OWNER_OBJECT),
				watches.ResourceFlavorByGK(ownerGroupKind),
			),
		)
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
