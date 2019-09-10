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
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"reflect"
	"time"

	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

func NamespaceSelection(namespace string) WatchSelectionFunction {
	return func(c Interface) (string, resources.TweakListOptionsFunc) {
		return namespace, nil
	}
}

///////////////////////////////////////////////////////////////////////////////

type configdef struct {
	name         string
	gotype       reflect.Type
	defaultValue interface{}
	desc         string
}

func (this *configdef) GetName() string {
	return this.name
}

func (this *configdef) Type() reflect.Type {
	return this.gotype
}

func (this *configdef) Default() interface{} {
	return this.defaultValue
}

func (this *configdef) Description() string {
	return this.desc
}

var _ OptionDefinition = &configdef{}

///////////////////////////////////////////////////////////////////////////////

type pooldef struct {
	name   string
	size   int
	period time.Duration
}

func (this *pooldef) GetName() string {
	return this.name
}
func (this *pooldef) Size() int {
	return this.size
}
func (this *pooldef) Period() time.Duration {
	return this.period
}

///////////////////////////////////////////////////////////////////////////////

type watchdef struct {
	rescdef
	reconciler string
	pool       string
}

type rescdef struct {
	rtype      ResourceKey
	selectFunc WatchSelectionFunction
}

func (this *rescdef) ResourceType() ResourceKey {
	return this.rtype
}
func (this *rescdef) WatchSelectionFunction() WatchSelectionFunction {
	return this.selectFunc
}

func (this *watchdef) Reconciler() string {
	return this.reconciler
}
func (this *watchdef) PoolName() string {
	return this.pool
}

///////////////////////////////////////////////////////////////////////////////

type cmddef struct {
	key        utils.Matcher
	reconciler string
	pool       string
}

func (this *cmddef) Key() utils.Matcher {
	return this.key
}
func (this *cmddef) Reconciler() string {
	return this.reconciler
}
func (this *cmddef) PoolName() string {
	return this.pool
}

type _Definition struct {
	name               string
	main               rescdef
	reconcilers        map[string]ReconcilerType
	watches            Watches
	commands           Commands
	resource_filters   []ResourceFilter
	required_clusters  []string
	require_lease      bool
	pools              map[string]PoolDefinition
	configs            map[string]OptionDefinition
	finalizerName      string
	finalizerDomain    string
	crds               map[string][]*CustomResourceDefinition
	activateExplicitly bool
}

var _ Definition = &_Definition{}

func (this *_Definition) String() string {
	s := fmt.Sprintf("controller %q:\n", this.name)
	s += fmt.Sprintf("  main rsc:    %s\n", this.main)
	s += fmt.Sprintf("  clusters:    %s\n", utils.Strings(this.RequiredClusters()...))
	s += fmt.Sprintf("  reconcilers: %s\n", toString(this.reconcilers))
	s += fmt.Sprintf("  watches:     %s\n", toString(this.watches))
	s += fmt.Sprintf("  commands:    %s\n", toString(this.commands))
	s += fmt.Sprintf("  pools:       %s\n", toString(this.pools))
	s += fmt.Sprintf("  finalizer:   %s\n", this.FinalizerName())
	return s
}

func (this *_Definition) GetName() string {
	return this.name
}
func (this *_Definition) MainResource() ResourceKey {
	return this.main.ResourceType()
}
func (this *_Definition) MainWatchResource() WatchResource {
	return &this.main
}
func (this *_Definition) Watches() Watches {
	return this.watches
}
func (this *_Definition) Commands() Commands {
	return this.commands
}
func (this *_Definition) ResourceFilters() []ResourceFilter {
	return this.resource_filters
}
func (this *_Definition) RequiredClusters() []string {
	if len(this.required_clusters) > 0 {
		return this.required_clusters
	}
	return []string{cluster.DEFAULT}
}

func (this *_Definition) RequireLease() bool {
	return this.require_lease
}
func (this *_Definition) FinalizerName() string {
	if this.finalizerName == "" {
		if this.finalizerDomain == "" {
			return "acme.com" + "/" + this.GetName()
		}
		return this.finalizerDomain + "/" + this.GetName()
	}
	return this.finalizerName
}

func (this *_Definition) CustomResourceDefinitions() map[string][]*CustomResourceDefinition {
	crds := map[string][]*CustomResourceDefinition{}
	for n, l := range this.crds {
		crds[n] = append([]*CustomResourceDefinition{}, l...)
	}
	return this.crds
}

func (this *_Definition) Reconcilers() map[string]ReconcilerType {
	types := map[string]ReconcilerType{}
	for n, d := range this.reconcilers {
		types[n] = d
	}
	return types
}
func (this *_Definition) Pools() map[string]PoolDefinition {
	pools := map[string]PoolDefinition{}
	for n, d := range this.pools {
		pools[n] = d
	}
	if len(pools) == 0 {
		pools[DEFAULT_POOL] = &pooldef{DEFAULT_POOL, 5, 30 * time.Second}
	}
	return pools
}
func (this *_Definition) ConfigOptions() map[string]OptionDefinition {
	cfgs := map[string]OptionDefinition{}
	for n, d := range this.configs {
		cfgs[n] = d
	}
	return cfgs
}

func (this *_Definition) ActivateExplicitly() bool {
	return this.activateExplicitly
}

////////////////////////////////////////////////////////////////////////////////

type Configuration struct {
	settings _Definition
	cluster  string
	pool     string
}

func Configure(name string) Configuration {
	return Configuration{
		settings: _Definition{
			name:        name,
			reconcilers: map[string]ReconcilerType{},
			pools:       map[string]PoolDefinition{},
			configs:     map[string]OptionDefinition{},
		},
		cluster: CLUSTER_MAIN,
		pool:    DEFAULT_POOL,
	}
}

func (this Configuration) Name(name string) Configuration {
	this.settings.name = name
	return this
}

func (this Configuration) MainResource(group, kind string, sel ...WatchSelectionFunction) Configuration {
	return this.MainResourceByKey(NewResourceKey(group, kind), sel...)
}

func (this Configuration) MainResourceByKey(key ResourceKey, sel ...WatchSelectionFunction) Configuration {
	this.settings.main.rtype = key
	if len(sel) > 0 {
		this.settings.main.selectFunc = sel[0]
	}
	return this
}

func (this Configuration) DefaultWorkerPool(size int, period time.Duration) Configuration {
	return this.WorkerPool(DEFAULT_POOL, size, period)
}

func (this Configuration) WorkerPool(name string, size int, period time.Duration) Configuration {
	if this.settings.pools[name] != nil {
		panic(fmt.Sprintf("pool %q already defined", name))
	}

	this.settings.pools[name] = &pooldef{name, size, period}
	this.pool = name
	return this
}

func (this Configuration) Pool(name string) Configuration {
	this.pool = name
	return this
}

func (this Configuration) Cluster(name string) Configuration {
	this.cluster = name
	this.pool = DEFAULT_POOL
	if name != CLUSTER_MAIN {
		for i, n := range this.settings.required_clusters {
			if n == name {
				if i == 0 {
					this.cluster = CLUSTER_MAIN
				}
				return this
			}
		}
		this.settings.required_clusters = append([]string{}, this.settings.required_clusters...)
		this.settings.required_clusters = append(this.settings.required_clusters, name)
	}
	return this
}

func (this Configuration) CustomResourceDefinitions(crds ...*apiext.CustomResourceDefinition) Configuration {
	m := map[string][]*CustomResourceDefinition{}
	for k, v := range this.settings.crds {
		m[k] = v
	}
	list := m[this.cluster]
	if list == nil {
		list = []*CustomResourceDefinition{}
	}
	list = append([]*CustomResourceDefinition{}, list...)
	for _, crd := range crds {
		vers := NewCustomResourceDefinition(crd)
		m[this.cluster] = append(list, vers)
	}
	this.settings.crds = m
	return this
}

func (this Configuration) VersionedCustomResourceDefinitions(crds ...*CustomResourceDefinition) Configuration {
	m := map[string][]*CustomResourceDefinition{}
	for k, v := range this.settings.crds {
		m[k] = v
	}
	list := m[this.cluster]
	if list == nil {
		list = []*CustomResourceDefinition{}
	}
	list = append([]*CustomResourceDefinition{}, list...)

	m[this.cluster] = append(list, crds...)
	this.settings.crds = m
	return this
}

func (this *Configuration) assureWatches() {
	if this.settings.watches == nil {
		this.settings.watches = map[string][]Watch{}
	}
}

func (this Configuration) Watches(keys ...ResourceKey) Configuration {
	return this.ReconcilerWatches(DEFAULT_RECONCILER, keys...)
}
func (this Configuration) SelectedWatches(sel WatchSelectionFunction, keys ...ResourceKey) Configuration {
	return this.ReconcilerSelectedWatches(DEFAULT_RECONCILER, sel, keys...)
}

func (this Configuration) Watch(group, kind string) Configuration {
	return this.ReconcilerWatches(DEFAULT_RECONCILER, NewResourceKey(group, kind))
}
func (this Configuration) SelectedWatch(sel WatchSelectionFunction, group, kind string) Configuration {
	return this.ReconcilerSelectedWatches(DEFAULT_RECONCILER, sel, NewResourceKey(group, kind))
}

func (this Configuration) ReconcilerWatch(reconciler, group, kind string) Configuration {
	return this.ReconcilerWatches(reconciler, NewResourceKey(group, kind))
}

func (this Configuration) ReconcilerWatches(reconciler string, keys ...ResourceKey) Configuration {
	this.assureWatches()
	for _, key := range keys {
		//logger.Infof("adding watch for %q:%q to pool %q", this.cluster, key, this.pool)
		this.settings.watches[this.cluster] = append(this.settings.watches[this.cluster], &watchdef{rescdef{key, nil}, reconciler, this.pool})
	}
	return this
}

func (this Configuration) ReconcilerSelectedWatches(reconciler string, sel WatchSelectionFunction, keys ...ResourceKey) Configuration {
	this.assureWatches()
	for _, key := range keys {
		//logger.Infof("adding watch for %q:%q to pool %q", this.cluster, key, this.pool)
		this.settings.watches[this.cluster] = append(this.settings.watches[this.cluster], &watchdef{rescdef{key, sel}, reconciler, this.pool})
	}
	return this
}

func (this Configuration) ActivateExplicitly() Configuration {
	this.settings.activateExplicitly = true
	return this
}

func (this *Configuration) assureCommands() {
	if this.settings.commands == nil {
		this.settings.commands = map[string][]Command{}
	}
}

func (this Configuration) Commands(cmd ...string) Configuration {
	return this.ReconcilerCommands(DEFAULT_RECONCILER, cmd...)
}

func (this Configuration) CommandMatchers(cmd ...utils.Matcher) Configuration {
	return this.ReconcilerCommandMatchers(DEFAULT_RECONCILER, cmd...)
}

func (this Configuration) ReconcilerCommands(reconciler string, cmd ...string) Configuration {
	this.assureCommands()
	for _, cmd := range cmd {
		this.settings.commands[reconciler] = append(this.settings.commands[reconciler], &cmddef{utils.NewStringMatcher(cmd), reconciler, this.pool})
	}
	return this
}
func (this Configuration) ReconcilerCommandMatchers(reconciler string, cmd ...utils.Matcher) Configuration {
	this.assureCommands()
	for _, cmd := range cmd {
		this.settings.commands[reconciler] = append(this.settings.commands[reconciler], &cmddef{cmd, reconciler, this.pool})
	}
	return this
}

func (this Configuration) Reconciler(t ReconcilerType, name ...string) Configuration {
	if len(name) == 0 {
		this.settings.reconcilers[DEFAULT_RECONCILER] = t
	} else {
		for _, n := range name {
			this.settings.reconcilers[n] = t
		}
	}
	return this
}

func (this Configuration) FinalizerName(name string) Configuration {
	this.settings.finalizerName = name
	return this
}

func (this Configuration) FinalizerDomain(name string) Configuration {
	this.settings.finalizerDomain = name
	return this
}
func (this Configuration) RequireLease() Configuration {
	this.settings.require_lease = true
	return this
}

func (this Configuration) StringOption(name string, desc string) Configuration {
	return this.addOption(name, reflect.TypeOf((*string)(nil)).Elem(), nil, desc)
}
func (this Configuration) StringArrayOption(name string, desc string) Configuration {
	return this.addOption(name, reflect.TypeOf(([]string)(nil)), nil, desc)
}
func (this Configuration) DefaultedStringOption(name, def string, desc string) Configuration {
	return this.addOption(name, reflect.TypeOf((*string)(nil)).Elem(), &def, desc)
}

func (this Configuration) IntOption(name string, desc string) Configuration {
	return this.addOption(name, reflect.TypeOf((*int)(nil)).Elem(), nil, desc)
}
func (this Configuration) DefaultedIntOption(name string, def int, desc string) Configuration {
	return this.addOption(name, reflect.TypeOf((*int)(nil)).Elem(), &def, desc)
}

func (this Configuration) BoolOption(name string, desc string) Configuration {
	return this.addOption(name, reflect.TypeOf((*bool)(nil)).Elem(), nil, desc)
}
func (this Configuration) DefaultedBoolOption(name string, def bool, desc string) Configuration {
	return this.addOption(name, reflect.TypeOf((*bool)(nil)).Elem(), &def, desc)
}

func (this Configuration) DurationOption(name string, desc string) Configuration {
	return this.addOption(name, reflect.TypeOf((*time.Duration)(nil)).Elem(), nil, desc)
}
func (this Configuration) DefaultedDurationOption(name string, def time.Duration, desc string) Configuration {
	return this.addOption(name, reflect.TypeOf((*time.Duration)(nil)).Elem(), &def, desc)
}

func (this Configuration) addOption(name string, t reflect.Type, def interface{}, desc string) Configuration {
	if this.settings.configs[name] != nil {
		panic(fmt.Sprintf("option %q already defined", name))
	}
	this.settings.configs[name] = &configdef{name, t, def, desc}
	return this
}

func (this Configuration) AddFilters(f ...ResourceFilter) Configuration {
	this.settings.resource_filters = append(this.settings.resource_filters, f...)
	return this
}
func (this Configuration) Filters(f ...ResourceFilter) Configuration {
	this.settings.resource_filters = f
	return this
}

func (this Configuration) Definition() Definition {
	return &this.settings
}

func (this Configuration) RegisterAt(registry RegistrationInterface, group ...string) error {
	return registry.RegisterController(this, group...)
}

func (this Configuration) MustRegisterAt(registry RegistrationInterface, group ...string) Configuration {
	registry.MustRegisterController(this, group...)
	return this
}

func (this Configuration) Register(group ...string) error {
	return registry.RegisterController(this, group...)
}

func (this Configuration) MustRegister(group ...string) Configuration {
	registry.MustRegisterController(this, group...)
	return this
}
