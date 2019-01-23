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
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile/reconcilers"
	"github.com/gardener/controller-manager-library/pkg/resources"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/crds"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"time"
)

const CONTROLLER_GROUP_DNS_SOURCES = "dnssources"
const TARGET_CLUSTER = "target"

const DNS_ANNOTATION = "dns.gardener.cloud/dnsnames"
const KEY_ANNOTATION = "dns.gardener.cloud/key"
const TTL_ANNOTATION = "dns.gardener.cloud/TTL"
const PERIOD_ANNOTATION = "dns.gardener.cloud/cname-lookup-interval"

const OPT_EXCLUDE = "exclude-domains"
const OPT_KEY = "key"
const OPT_NAMESPACE = "target-namespace"
const OPT_NAMEPREFIX = "target-name-prefix"

var ENTRY = resources.NewGroupKind(api.GroupName, api.DNSEntryKind)

func init() {
	cluster.Register(TARGET_CLUSTER, "target", "target cluster for dns requests")
}

func DNSSourceController(source DNSSourceType, reconcilerType controller.ReconcilerType) controller.Configuration {
	gk := source.GroupKind()
	return controller.Configure(source.Name()).
		StringArrayOption(OPT_EXCLUDE, "excluded domains").
		StringOption(OPT_KEY, "selecting key for annotation").
		DefaultedStringOption(OPT_NAMESPACE, "", "target namespace for cross cluster generation").
		DefaultedStringOption(OPT_NAMEPREFIX, "", "name prefix in target namespace for cross cluster generation").
		FinalizerDomain("mandelsoft.org").
		Reconciler(SourceReconciler(source, reconcilerType)).
		Cluster(cluster.DEFAULT). // first one used as MAIN cluster
		DefaultWorkerPool(2, 120*time.Second).
		MainResource(gk.Group, gk.Kind).
		Reconciler(reconcilers.SlaveReconcilerType(source.Name(), SlaveResources, SlaveReconcilerType, MasterResourcesType(source.GroupKind())), "entries").
		Cluster(TARGET_CLUSTER).
		CustomResourceDefinitions(crds.DNSEntryCRD).
		WorkerPool("targets", 2, 0).
		ReconcilerWatch("entries", api.GroupName, api.DNSEntryKind)
}

func SlaveResources(c controller.Interface) []resources.Interface {
	target := c.GetCluster(TARGET_CLUSTER)
	res, err := target.Resources().GetByGK(ENTRY)
	if err != nil {
		panic(err)
	}
	return []resources.Interface{res}
}

func MasterResourcesType(kind schema.GroupKind) reconcilers.Resources {
	return func(c controller.Interface) []resources.Interface {
		target := c.GetMainCluster()
		res, err := target.Resources().GetByGK(kind)
		if err != nil {
			panic(err)
		}
		return []resources.Interface{res}
	}
}
