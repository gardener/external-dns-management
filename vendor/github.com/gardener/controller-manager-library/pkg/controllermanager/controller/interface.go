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
	"context"
	"fmt"
	"reflect"
	"time"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type ReconcilerType func(Interface) (reconcile.Interface, error)

type Pool interface {
	StartTicker()
	EnqueueCommand(name string)
	EnqueueCommandRateLimited(name string)
	EnqueueCommandAfter(name string, duration time.Duration)
	Period() time.Duration
}

type Interface interface {
	GetName() string
	IsReady() bool
	Owning() ResourceKey
	GetContext() context.Context
	GetEnvironment() Environment
	GetPool(name string) Pool
	GetMainCluster() cluster.Interface
	GetClusterById(id string) cluster.Interface
	GetCluster(name string) cluster.Interface
	GetClusterAliases(eff string) utils.StringSet
	GetDefinition() Definition

	GetOption(name string) (*config.ArbitraryOption, error)
	GetStringOption(name string) (string, error)
	GetIntOption(name string) (int, error)
	GetDurationOption(name string) (time.Duration, error)
	GetBoolOption(name string) (bool, error)
	GetStringArrayOption(name string) ([]string, error)

	GetSharedValue(key interface{}) interface{}
	GetOrCreateSharedValue(key interface{}, create func() interface{}) interface{}

	HasFinalizer(obj resources.Object) bool
	SetFinalizer(obj resources.Object) error
	RemoveFinalizer(obj resources.Object) error
	FinalizerHandler() Finalizer
	SetFinalizerHandler(Finalizer)

	EnqueueKey(key resources.ClusterObjectKey) error
	Enqueue(object resources.Object) error
	EnqueueRateLimited(object resources.Object) error
	EnqueueAfter(object resources.Object, duration time.Duration) error
	EnqueueCommand(cmd string) error

	logger.LogContext

	GetObject(key resources.ClusterObjectKey) (resources.Object, error)
	GetCachedObject(key resources.ClusterObjectKey) (resources.Object, error)
}

type Watch interface {
	ResourceType() ResourceKey
	Reconciler() string
	PoolName() string
}
type Command interface {
	Key() utils.Matcher
	Reconciler() string
	PoolName() string
}

// ResourceKey implementations are used as key and MUST therefore be value types
type ResourceKey interface {
	GroupKind() schema.GroupKind
	String() string
}

type resourceKey struct {
	key schema.GroupKind
}

func NewResourceKey(group, kind string) ResourceKey {
	if group == "core" {
		group = corev1.GroupName
	}
	return resourceKey{schema.GroupKind{Group: group, Kind: kind}}
}
func (k resourceKey) GroupKind() schema.GroupKind {
	return k.key
}
func (k resourceKey) String() string {
	if k.key.Group == corev1.GroupName {
		return fmt.Sprintf("%s/%s", "core", k.key.Kind)

	}
	return fmt.Sprintf("%s/%s", k.key.Group, k.key.Kind)
}

func GetResourceKey(obj resources.Object) ResourceKey {
	return NewResourceKey(obj.GroupKind().Group, obj.GroupKind().Kind)
}

type Watches map[string][]Watch
type Commands map[string][]Command

const CLUSTER_MAIN = mappings.CLUSTER_MAIN
const DEFAULT_POOL = "default"
const DEFAULT_RECONCILER = "default"

type PoolDefinition interface {
	GetName() string
	Size() int
	Period() time.Duration
}

type OptionDefinition interface {
	GetName() string
	Type() reflect.Type
	Default() interface{}
	Description() string
}

type Definition interface {
	GetName() string
	//Create(Object) (Reconciler, error)
	Reconcilers() map[string]ReconcilerType
	MainResource() ResourceKey
	Watches() Watches
	Commands() Commands
	Pools() map[string]PoolDefinition
	ResourceFilters() []ResourceFilter
	RequiredClusters() []string
	CustomResourceDefinitions() map[string][]*v1beta1.CustomResourceDefinition
	RequireLease() bool
	FinalizerName() string

	ConfigOptions() map[string]OptionDefinition

	Definition() Definition

	String() string
}
