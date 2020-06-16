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

package controller

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

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
	RequiredControllers() []string
	CustomResourceDefinitions() map[string][]*apiextensions.CustomResourceDefinitionVersions
	RequireLease() bool
	FinalizerName() string
	ActivateExplicitly() bool
	ConfigOptions() map[string]OptionDefinition
	ConfigOptionSources() extension.OptionSourceDefinitions

	Scheme() *runtime.Scheme

	Definition() Definition

	String() string
}
