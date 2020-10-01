/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/controller/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type ReconcilerType func(Interface) (reconcile.Interface, error)

type Environment interface {
	extension.Environment
	SharedAttributes

	GetConfig() *areacfg.Config
	Enqueue(obj resources.Object)
	EnqueueKey(key resources.ClusterObjectKey)
}

type Pool interface {
	StartTicker()
	EnqueueCommand(name string)
	EnqueueCommandRateLimited(name string)
	EnqueueCommandAfter(name string, duration time.Duration)
	Period() time.Duration
}

type Interface interface {
	extension.ElementBase
	SharedAttributes

	IsReady() bool
	Owning() ResourceKey
	GetMainWatchResource() WatchResource
	GetEnvironment() Environment
	GetPool(name string) Pool
	GetMainCluster() cluster.Interface
	GetClusterById(id string) cluster.Interface
	GetCluster(name string) cluster.Interface
	GetClusterAliases(eff string) utils.StringSet
	GetDefinition() Definition

	HasFinalizer(obj resources.Object) bool
	SetFinalizer(obj resources.Object) error
	RemoveFinalizer(obj resources.Object) error
	FinalizerHandler() Finalizer
	SetFinalizerHandler(Finalizer)

	Synchronize(log logger.LogContext, name string, initiator resources.Object) (bool, error)

	EnqueueKey(key resources.ClusterObjectKey) error
	Enqueue(object resources.Object) error
	EnqueueRateLimited(object resources.Object) error
	EnqueueAfter(object resources.Object, duration time.Duration) error
	EnqueueCommand(cmd string) error

	GetObject(key resources.ClusterObjectKey) (resources.Object, error)
	GetCachedObject(key resources.ClusterObjectKey) (resources.Object, error)
}

type WatchSelectionFunction func(c Interface) (string, resources.TweakListOptionsFunc)

type WatchResource interface {
	ResourceType() ResourceKey
	WatchSelectionFunction() WatchSelectionFunction
}

type Watch interface {
	WatchResource
	Reconciler() string
	PoolName() string
}
type Command interface {
	Key() utils.Matcher
	Reconciler() string
	PoolName() string
}

// ResourceKey implementations are used as key and MUST therefore be value types
type ResourceKey = extension.ResourceKey

func NewResourceKey(group, kind string) ResourceKey {
	return extension.NewResourceKey(group, kind)
}

func NewResourceKeyByGK(gk schema.GroupKind) ResourceKey {
	return extension.NewResourceKey(gk.Group, gk.Kind)
}

func GetResourceKey(objspec interface{}) ResourceKey {
	return extension.GetResourceKey(objspec)
}

// ClusterResourceKey implementations are used as key and MUST therefore be value types
type ClusterResourceKey extension.ResourceKey

func NewClusterResourceKey(clusterid, group, kind string) ResourceKey {
	return extension.NewClusterResourceKey(clusterid, group, kind)
}

func GetClusterResourceKey(objspec interface{}) ClusterResourceKey {
	return extension.GetClusterResourceKey(objspec)
}

type Watches map[string][]Watch
type Commands map[string][]Command

const CLUSTER_MAIN = mappings.CLUSTER_MAIN
const DEFAULT_POOL = "default"
const DEFAULT_RECONCILER = "default"

type SyncerDefinition interface {
	GetName() string
	GetCluster() string
	GetResource() ResourceKey
}

type PoolDefinition interface {
	GetName() string
	Size() int
	Period() time.Duration
}

type OptionDefinition extension.OptionDefinition

type ForeignClusterRefs interface {
	From() string
	To() utils.StringSet
	String() string

	Add(names ...string) ForeignClusterRefs
	AddSet(sets ...utils.StringSet) ForeignClusterRefs
}

type CrossClusterRefs map[string]ForeignClusterRefs

func (this CrossClusterRefs) String() string {
	r := "{"
	sep := ""
	for _, m := range this {
		r = fmt.Sprintf("%s%s%s", r, sep, m)
		sep = ", "
	}
	return r + "}"
}

func (this CrossClusterRefs) AddAll(refs CrossClusterRefs) {
	for _, r := range refs {
		this.Add(r)
	}
}

func (this CrossClusterRefs) Add(ref ForeignClusterRefs) {
	if ref != nil {
		c := this[ref.From()]
		if c == nil {
			c = NewForeignClusterRefs(ref.From())
			this[ref.From()] = c
		}
		for r := range ref.To() {
			c.Add(r)
		}
	}
}

func (this CrossClusterRefs) Targets() utils.StringSet {
	targets := utils.StringSet{}
	for _, r := range this {
		targets.AddSet(r.To())
	}
	return targets
}

func (this CrossClusterRefs) Map(mapping controllermanager.Mapping) CrossClusterRefs {
	if this == nil {
		return nil
	}
	result := CrossClusterRefs{}
	for _, cross := range this {
		from := mapping.Map(cross.From())
		if from == "" {
			panic(fmt.Sprintf("programmatic error: there must always be a mapping for cluster mentioned in the cross cluster references %s: %s", cross, cross.From()))
		}
		for n := range cross.To() {
			m := mapping.Map(n)
			if m == "" {
				panic(fmt.Sprintf("programmatic error: there must always be a mapping for cluster mentioned in the cross cluster references %s: %s", cross, n))
			}
			if m != from {
				result.Add(NewForeignClusterRefs(from).Add(m))
			}
		}
	}
	return result
}

type Definition interface {
	extension.OrderedElem

	// Create(Object) (Reconciler, error)
	Reconcilers() map[string]ReconcilerType
	Syncers() map[string]SyncerDefinition
	MainResource() ResourceKey
	MainWatchResource() WatchResource
	Watches() Watches
	Commands() Commands
	Pools() map[string]PoolDefinition
	ResourceFilters() []ResourceFilter
	RequiredClusters() []string
	CrossClusterReferences() CrossClusterRefs
	RequiredControllers() []string
	CustomResourceDefinitions() map[string][]*apiextensions.CustomResourceDefinitionVersions
	RequireLease() bool
	LeaseClusterName() string
	FinalizerName() string
	ActivateExplicitly() bool
	ConfigOptions() map[string]OptionDefinition
	ConfigOptionSources() extension.OptionSourceDefinitions

	Scheme() *runtime.Scheme

	Definition() Definition

	String() string
}
