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
	"github.com/gardener/external-dns-management/pkg/dns"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile/reconcilers"
	"github.com/gardener/controller-manager-library/pkg/resources"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/crds"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const CONTROLLER_GROUP_DNS_SOURCES = "dnssources"
const TARGET_CLUSTER = "target"

const DNS_ANNOTATION = "dns.gardener.cloud/dnsnames"
const TTL_ANNOTATION = "dns.gardener.cloud/ttl"
const PERIOD_ANNOTATION = "dns.gardener.cloud/cname-lookup-interval"
const CLASS_ANNOTATION = dns.CLASS_ANNOTATION

const OPT_CLASS = "dns-class"
const OPT_TARGET_CLASS = "dns-target-class"
const OPT_EXCLUDE = "exclude-domains"
const OPT_KEY = "key"
const OPT_NAMESPACE = "target-namespace"
const OPT_NAMEPREFIX = "target-name-prefix"
const OPT_TARGET_CREATOR_LABEL_NAME = "target-creator-label-name"
const OPT_TARGET_CREATOR_LABEL_VALUE = "target-creator-label-value"
const OPT_TARGET_OWNER_ID = "target-owner-id"
const OPT_TARGET_SET_IGNORE_OWNERS = "target-set-ignore-owners"

var ENTRY = resources.NewGroupKind(api.GroupName, api.DNSEntryKind)

func init() {
	cluster.Register(TARGET_CLUSTER, "target", "target cluster for dns requests")
}

func DNSSourceController(source DNSSourceType, reconcilerType controller.ReconcilerType) controller.Configuration {
	gk := source.GroupKind()
	return controller.Configure(source.Name()).
		DefaultedStringOption(OPT_CLASS, dns.DEFAULT_CLASS, "identifier used to differentiate responsible controllers for entries").
		StringOption(OPT_TARGET_CLASS, "identifier used to differentiate responsible dns controllers for target entries").
		StringArrayOption(OPT_EXCLUDE, "excluded domains").
		StringOption(OPT_KEY, "selecting key for annotation").
		DefaultedStringOption(OPT_NAMESPACE, "", "target namespace for cross cluster generation").
		DefaultedStringOption(OPT_NAMEPREFIX, "", "name prefix in target namespace for cross cluster generation").
		DefaultedStringOption(OPT_TARGET_CREATOR_LABEL_NAME, "creator", "label name to store the creator for generated DNS entries").
		StringOption(OPT_TARGET_CREATOR_LABEL_VALUE, "label value for creator label").
		StringOption(OPT_TARGET_OWNER_ID, "owner id to use for generated DNS entries").
		BoolOption(OPT_TARGET_SET_IGNORE_OWNERS, "mark generated DNS entries to omit owner based access control").
		FinalizerDomain(api.GroupName).
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

var SlaveResources = reconcilers.ClusterResources(TARGET_CLUSTER, ENTRY)

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
